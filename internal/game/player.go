package game

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"tap/internal/logging"
	"tap/internal/world"
)

// QuestProgress tracks one quest for one player. Count only grows for "defeat" objectives;
// fetch and deliver are measured against the inventory when the quest is turned in.
type QuestProgress struct {
	Count     int
	Completed bool
}

// initPlayer gives a freshly connected player the character it picked, its stats and starting gear.
func (h *Hub) initPlayer(c *Client, char *world.Character) {
	c.Char = char
	c.Level = 1
	c.Gold = h.world.Config.Economy.StartingGold
	c.Statuses = map[string]int{}
	c.Buffs = map[world.StatName]*buff{}
	c.Equipped = map[world.EquipSlot]string{}
	c.Quests = map[string]*QuestProgress{}
	c.talkIndex = map[string]int{}
	c.seen = map[string]bool{}
	h.unlockMoves(c)

	stats := h.statsOf(c)
	c.HP, c.Mana = stats.HP, stats.Mana

	for _, stack := range c.Char.StartingItems {
		for i := 0; i < stack.Count; i++ {
			h.newItem(stack.Item, "", c)
		}
	}
}

// statsOf is the player's current maximum stats: archetype base plus linear growth per level,
// plus the bonuses of the equipment it wears.
func (h *Hub) statsOf(c *Client) world.Stats {
	s := h.world.Archetypes[c.Char.Archetype].StatsAt(c.Level)
	for _, inst := range h.equipped(c) {
		s = s.Add(inst.Def.Equipment.Bonus)
	}
	return s
}

// grantXP adds experience and levels the player up as many times as the total allows.
// A level up restores health and mana in full.
func (h *Hub) grantXP(c *Client, xp int) []string {
	cfg := h.world.Config.Leveling
	c.XP += xp
	var lines []string
	for c.Level < cfg.MaxLevel && c.XP >= cfg.XPToNext(c.Level) {
		c.XP -= cfg.XPToNext(c.Level)
		c.Level++
		stats := h.statsOf(c)
		c.HP, c.Mana = stats.HP, stats.Mana
		lines = append(lines, fmt.Sprintf("%s reached level %d", c.Name, c.Level))
	}
	return append(lines, h.unlockMoves(c)...)
}

// respawn sends a defeated player back to the safe room with reduced health,
// clears its ailments and takes a share of its gold.
func (h *Hub) respawn(c *Client) {
	cfg := h.world.Config
	stats := h.statsOf(c)

	c.HP = max(1, stats.HP*cfg.RespawnHPPercent/100)
	c.Mana = stats.Mana
	c.Statuses = map[string]int{}
	c.Buffs = map[world.StatName]*buff{}
	c.Gold -= c.Gold * cfg.Economy.DeathGoldLossPercent / 100

	h.log.Warn("player_defeated", logging.Fields{
		"player": c.Name, "room": c.Room, "respawn_room": cfg.RespawnRoom, "hp": c.HP, "gold": c.Gold,
	})

	from := c.Room
	c.Room = cfg.RespawnRoom
	if from != c.Room {
		h.broadcastRoom(from, "EVT ROOM PRESENCE LEAVE "+c.Name, c)
		h.broadcastRoom(c.Room, "EVT ROOM PRESENCE ENTER "+c.Name, c)
		h.notifyGroupMove(c)
	}
}

// statusLabel summarises the player's condition for the STATUS command.
func (c *Client) statusLabel() string {
	for _, id := range sortedKeys(c.Statuses) {
		return id // one ailment is enough to describe the player
	}
	if c.HP <= 0 {
		return "defeated"
	}
	return "healthy"
}

// characterInfo describes a playable character for the selection screen (CHARACTERS command).
type characterInfo struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Class            string      `json:"class"`
	ClassDescription string      `json:"class_description"`
	Element          string      `json:"element"`
	Stats            world.Stats `json:"stats"`
	Default          bool        `json:"default,omitempty"`
}

// characters answers CHARACTERS: the list of characters, sorted by class then by name.
func (h *Hub) characters(c *Client) {
	list := []characterInfo{}
	for _, id := range sortedKeys(h.world.Characters) {
		ch := h.world.Characters[id]
		a := h.world.Archetypes[ch.Archetype]
		list = append(list, characterInfo{
			ID: ch.ID, Name: ch.Name, Description: ch.Description,
			Class: a.Name, ClassDescription: a.Description, Element: string(ch.Type),
			Stats: a.BaseStats, Default: ch.ID == h.world.Config.DefaultCharacter,
		})
	}
	slices.SortStableFunc(list, func(a, b characterInfo) int {
		return cmp.Or(cmp.Compare(a.Class, b.Class), cmp.Compare(a.Name, b.Name))
	})
	h.sendJSON(c, list)
}

// findCharacter finds a character by its id ("char.pyra") or its name ("Pyra").
func (h *Hub) findCharacter(query string) *world.Character {
	if ch, ok := h.world.Characters[query]; ok {
		return ch
	}
	for _, id := range sortedKeys(h.world.Characters) {
		if ch := h.world.Characters[id]; strings.EqualFold(ch.Name, query) || strings.EqualFold(id, "char."+query) {
			return ch
		}
	}
	return nil
}

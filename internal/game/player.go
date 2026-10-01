package game

import (
	"fmt"

	"tap/internal/logging"
	"tap/internal/world"
)

// QuestProgress tracks one quest for one player. Count only grows for "defeat" objectives;
// fetch and deliver are measured against the inventory when the quest is turned in.
type QuestProgress struct {
	Count     int
	Completed bool
}

// initPlayer gives a freshly connected player its character, stats and starting gear.
// Every player uses the default character for now; picking a class is part of the bonus layer.
func (h *Hub) initPlayer(c *Client) {
	c.Char = h.world.Characters[h.world.Config.DefaultCharacter]
	c.Level = 1
	c.Gold = h.world.Config.Economy.StartingGold
	c.Statuses = map[string]int{}
	c.Quests = map[string]*QuestProgress{}
	c.talkIndex = map[string]int{}

	stats := h.statsOf(c)
	c.HP, c.Mana = stats.HP, stats.Mana

	for _, stack := range c.Char.StartingItems {
		for i := 0; i < stack.Count; i++ {
			h.newItem(stack.Item, "", c)
		}
	}
}

// statsOf is the player's current maximum stats: archetype base plus linear growth per level.
func (h *Hub) statsOf(c *Client) world.Stats {
	return h.world.Archetypes[c.Char.Archetype].StatsAt(c.Level)
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
	return lines
}

// respawn sends a defeated player back to the safe room with reduced health,
// clears its ailments and takes a share of its gold.
func (h *Hub) respawn(c *Client) {
	cfg := h.world.Config
	stats := h.statsOf(c)

	c.HP = max(1, stats.HP*cfg.RespawnHPPercent/100)
	c.Mana = stats.Mana
	c.Statuses = map[string]int{}
	c.Gold -= c.Gold * cfg.Economy.DeathGoldLossPercent / 100

	h.log.Warn("player_defeated", logging.Fields{
		"player": c.Name, "room": c.Room, "respawn_room": cfg.RespawnRoom, "hp": c.HP, "gold": c.Gold,
	})

	from := c.Room
	c.Room = cfg.RespawnRoom
	if from != c.Room {
		h.broadcastRoom(from, "EVT ROOM PRESENCE LEAVE "+c.Name, c)
		h.broadcastRoom(c.Room, "EVT ROOM PRESENCE ENTER "+c.Name, c)
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

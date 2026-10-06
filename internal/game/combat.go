package game

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"tap/internal/logging"
	"tap/internal/world"
)

const (
	errNPCNotFound  = "ERR 404 NPC_NOT_FOUND"
	errNotHostile   = "ERR 405 NPC_NOT_HOSTILE"
	errPlayerDown   = "ERR 405 PLAYER_DEFEATED"
	basicAttackMove = "move.strike"
)

// attackReply follows RFC 42TAP 5.4.5. The extra fields are additions a plain RFC client ignores.
type attackReply struct {
	AttackerHP int      `json:"attacker_hp"`
	TargetHP   int      `json:"target_hp"`
	Damage     int      `json:"damage"`
	Status     string   `json:"status"`
	Target     string   `json:"target,omitempty"`
	Log        []string `json:"log,omitempty"`
	Move       string   `json:"move,omitempty"`
	Mana       int      `json:"mana"`
}

// statusReply follows RFC 42TAP 5.4.6, plus the fields our own clients use.
type statusReply struct {
	HP      int    `json:"hp"`
	MaxHP   int    `json:"max_hp"`
	Status  string `json:"status"`
	Mana    int    `json:"mana"`
	MaxMana int    `json:"max_mana"`
	Level   int    `json:"level"`
	XP      int    `json:"xp"`
	XPNext  int    `json:"xp_next"`
	Gold    int    `json:"gold"`
	Room    string `json:"room"`

	// The character being played and its attacks, so clients can display them.
	Character string            `json:"character"`
	Class     string            `json:"class"`
	Element   string            `json:"element"`
	Moves     []moveInfo        `json:"moves"`
	Equipment map[string]string `json:"equipment"` // slot -> item id
	Bag       []bagEntry        `json:"bag"`
}

// moveInfo describes one of the character's attacks: already learned, or unlocked at a later level.
type moveInfo struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Category    string  `json:"category"`
	Power       int     `json:"power"`
	ManaCost    int     `json:"mana_cost"`
	Accuracy    float64 `json:"accuracy"`
	Level       int     `json:"level"`
	Learned     bool    `json:"learned"`
}

func (h *Hub) status(c *Client) {
	stats := h.statsOf(c)
	reply := statusReply{
		HP: c.HP, MaxHP: stats.HP, Status: c.statusLabel(),
		Mana: c.Mana, MaxMana: stats.Mana,
		Level: c.Level, XP: c.XP, XPNext: h.world.Config.Leveling.XPToNext(c.Level),
		Gold: c.Gold, Room: c.Room,
		Character: c.Char.Name, Class: h.world.Archetypes[c.Char.Archetype].Name, Element: string(c.Char.Type),
		Moves: h.movesOf(c), Equipment: h.equipmentOf(c), Bag: h.bagOf(c),
	}
	h.sendJSON(c, reply)
}

// movesOf lists every attack the character can learn, sorted by required level.
func (h *Hub) movesOf(c *Client) []moveInfo {
	moves := []moveInfo{}
	for _, id := range sortedKeys(h.world.Moves) {
		m := h.world.Moves[id]
		if !m.AvailableTo(c.Char) {
			continue
		}
		moves = append(moves, moveInfo{
			ID: m.ID, Name: m.Name, Description: m.Description,
			Type: string(m.Type), Category: string(m.Category),
			Power: m.Power, ManaCost: h.manaCost(c, m), Accuracy: m.Accuracy,
			Level: m.Learn.Level, Learned: c.Level >= m.Learn.Level,
		})
	}
	slices.SortStableFunc(moves, func(a, b moveInfo) int { return a.Level - b.Level })
	return moves
}

func (h *Hub) sendJSON(c *Client, v any) {
	payload, err := json.Marshal(v)
	if err != nil {
		h.log.Error("encode_failed", logging.Fields{"player": c.Name, "error": err.Error()})
		c.send(errInternal)
		return
	}
	c.send("OK " + string(payload))
}

// reviveMonsters brings back creatures whose respawn delay has elapsed. It runs before every
// command instead of using a timer goroutine, which keeps the hub the only owner of the state.
func (h *Hub) reviveMonsters() {
	now := time.Now()
	for _, id := range sortedKeys(h.monsters) {
		m := h.monsters[id]
		if m.HP > 0 || m.RespawnSeconds <= 0 || now.Before(m.DeadUntil) {
			continue
		}
		m.HP = m.Def.Stats.HP
		h.broadcastRoom(m.Room, fmt.Sprintf("EVT ROOM SPAWN %s", m.Def.ID), nil)
	}
}

// findMonster looks for a living creature of the room, by id or display name.
func (h *Hub) findMonster(room, query string) *MonsterInstance {
	q := normalizeQuery(query)
	for _, id := range sortedKeys(h.monsters) {
		m := h.monsters[id]
		if m.Room != room || m.HP <= 0 {
			continue
		}
		defID := strings.ToLower(m.Def.ID)
		if q == defID || q == strings.ToLower(m.Def.Name) || "mob."+q == defID {
			return m
		}
	}
	return nil
}

// attack resolves one full round: initiative decides who strikes first, ailments tick at the end.
// "ATTACK <monster>" uses the basic attack; "ATTACK <monster> <move>" the chosen one.
func (h *Hub) attack(c *Client, args string) {
	query, moveID := splitAttack(args)
	if query == "" {
		c.send(errMissingArgument)
		return
	}
	if c.HP <= 0 {
		c.send(errPlayerDown)
		return
	}

	target := h.findMonster(c.Room, query)
	if target == nil {
		// A peaceful NPC of the room is a clearer error than "not found".
		if h.findNPC(c.Room, query) != nil {
			c.send(errNotHostile)
			return
		}
		c.send(errNPCNotFound)
		return
	}

	move, failure := h.chooseMove(c, moveID)
	if failure != "" {
		c.send(failure)
		return
	}
	c.Mana -= h.manaCost(c, move)

	stats := h.combatStats(c)
	var lines []string
	damage := 0
	playerFirst := stats.Speed >= target.Def.Stats.Speed

	if playerFirst {
		damage, lines = h.playerAct(c, target, move, stats, lines)
	}
	if target.HP > 0 {
		lines = h.monsterStrike(c, target, stats, lines)
	}
	if !playerFirst && c.HP > 0 && target.HP > 0 {
		damage, lines = h.playerAct(c, target, move, stats, lines)
	}

	lines = h.tickStatuses(c, target, lines)
	tickBuffs(c)

	status := "combat"
	switch {
	case c.HP <= 0:
		status = "defeated"
		lines = append(lines, fmt.Sprintf("%s was defeated by %s", c.Name, target.Def.Name))
		h.respawn(c)
	case target.HP <= 0:
		status = "victory"
		lines = append(lines, h.killMonster(c, target)...)
		c.Buffs = map[world.StatName]*buff{}
		if h.world.Config.Combat.RefillManaAfterCombat {
			c.Mana = h.statsOf(c).Mana
		}
	}

	h.log.Info("combat_round", logging.Fields{
		"player": c.Name, "room": c.Room, "monster": target.Def.ID, "move": move.ID,
		"damage": damage, "player_hp": max(0, c.HP), "monster_hp": max(0, target.HP),
		"status": status, "log": lines,
	})
	h.broadcastRoom(c.Room, fmt.Sprintf("EVT ROOM COMBAT %s %s %d", c.Name, target.Def.ID, damage), c)

	h.sendJSON(c, attackReply{
		AttackerHP: max(0, c.HP), TargetHP: max(0, target.HP),
		Damage: damage, Status: status, Target: target.Def.ID, Log: lines,
		Move: move.ID, Mana: c.Mana,
	})
}

func (h *Hub) monsterStrike(c *Client, m *MonsterInstance, stats world.Stats, lines []string) []string {
	if m.Statuses["status.sleep"] > 0 {
		return append(lines, fmt.Sprintf("%s is asleep", m.Def.Name))
	}
	if len(m.Def.Moves) == 0 {
		return lines
	}
	move := h.world.Moves[m.Def.Moves[rand.IntN(len(m.Def.Moves))]]
	if move == nil {
		return lines
	}
	if move.Category == world.MoveStatus {
		return h.applyEffects(move, c, m, append(lines, fmt.Sprintf("%s used %s", m.Def.Name, move.Name)))
	}

	damage, note := h.computeDamage(move, m.Def.Type, m.Def.Stats, c.Char.Type, stats)
	if damage == 0 {
		return append(lines, fmt.Sprintf("%s missed %s", m.Def.Name, c.Name))
	}
	c.HP -= damage
	lines = append(lines, fmt.Sprintf("%s hit %s with %s for %d%s", m.Def.Name, c.Name, move.Name, damage, note))
	return h.applyEffects(move, c, nil, lines)
}

// computeDamage: power scaled by attack against defence, then type effectiveness, same-type bonus
// and critical hits. A miss returns 0.
func (h *Hub) computeDamage(move *world.Move, atkType world.Element, atk world.Stats, defType world.Element, def world.Stats) (int, string) {
	if rand.Float64() > move.Accuracy {
		return 0, ""
	}
	offence, defence := atk.Attack, def.Defense
	if move.Category == world.MoveMagical {
		offence, defence = atk.MagicAttack, def.MagicDefense
	}

	cfg := h.world.Config.Combat
	value := float64(move.Power) * float64(offence) / float64(defence+10)

	note := ""
	if mult := h.world.Types.Multiplier(move.Type, defType); mult != 1 {
		value *= mult
		if mult > 1 {
			note = " (super effective)"
		} else {
			note = " (not very effective)"
		}
	}
	if move.Type == atkType {
		value *= cfg.STABMultiplier
	}
	if rand.Float64() < cfg.CritChance {
		value *= cfg.CritMultiplier
		note += " (critical)"
	}
	return max(1, int(math.Round(value))), note
}

// applyEffects applies a move's side effects. Only ailments and healing are handled here;
// stat buffs belong to the extended combat layer.
func (h *Hub) applyEffects(move *world.Move, player *Client, monster *MonsterInstance, lines []string) []string {
	for _, e := range move.Effects {
		if rand.Float64() > e.Chance {
			continue
		}
		switch e.Kind {
		case world.EffectStatus:
			def := h.world.Statuses[e.Status]
			if def == nil {
				continue
			}
			turns := e.Duration
			if turns == 0 {
				turns = def.DefaultDuration
			}
			if player != nil {
				player.Statuses[e.Status] = turns
				lines = append(lines, fmt.Sprintf("%s is %s", player.Name, def.Name))
			} else if monster != nil {
				monster.Statuses[e.Status] = turns
				lines = append(lines, fmt.Sprintf("%s is %s", monster.Def.Name, def.Name))
			}
		case world.EffectHealPercent:
			if monster != nil && e.Target == world.TargetSelf {
				heal := int(float64(monster.Def.Stats.HP) * e.Amount / 100)
				monster.HP = min(monster.Def.Stats.HP, monster.HP+heal)
				lines = append(lines, fmt.Sprintf("%s recovered %d HP", monster.Def.Name, heal))
			}
		}
	}
	return lines
}

// tickStatuses runs end-of-round ailments on both fighters.
func (h *Hub) tickStatuses(c *Client, m *MonsterInstance, lines []string) []string {
	stats := h.statsOf(c)
	for _, id := range sortedKeys(c.Statuses) {
		def := h.world.Statuses[id]
		if dmg := int(float64(stats.HP) * def.DamagePercent / 100); dmg > 0 {
			c.HP -= dmg
			lines = append(lines, fmt.Sprintf("%s lost %d HP to %s", c.Name, dmg, def.Name))
		}
		if heal := int(float64(stats.HP) * def.HealPercent / 100); heal > 0 {
			c.HP = min(stats.HP, c.HP+heal)
			lines = append(lines, fmt.Sprintf("%s recovered %d HP", c.Name, heal))
		}
		if c.Statuses[id]--; c.Statuses[id] <= 0 {
			delete(c.Statuses, id)
			lines = append(lines, fmt.Sprintf("%s is no longer %s", c.Name, def.Name))
		}
	}
	if m.HP <= 0 {
		return lines
	}
	for _, id := range sortedKeys(m.Statuses) {
		def := h.world.Statuses[id]
		if dmg := int(float64(m.Def.Stats.HP) * def.DamagePercent / 100); dmg > 0 {
			m.HP -= dmg
			lines = append(lines, fmt.Sprintf("%s lost %d HP to %s", m.Def.Name, dmg, def.Name))
		}
		if m.Statuses[id]--; m.Statuses[id] <= 0 {
			delete(m.Statuses, id)
		}
	}
	return lines
}

// killMonster hands out experience, gold and loot, and advances "defeat" quests.
func (h *Hub) killMonster(c *Client, m *MonsterInstance) []string {
	m.HP = 0
	m.Statuses = map[string]int{}
	m.DeadUntil = time.Now().Add(time.Duration(m.RespawnSeconds) * time.Second)

	lines := []string{fmt.Sprintf("%s defeated %s", c.Name, m.Def.Name)}
	c.Gold += m.Def.Gold

	// Group share: the members present in the room each receive an equal share of the XP,
	// with +10% per ally to reward playing together. The gold stays with the winner.
	party := h.alliesHere(c, true)
	share := m.Def.XP
	if len(party) > 1 {
		share = int(math.Ceil(float64(m.Def.XP) * (1 + 0.1*float64(len(party)-1)) / float64(len(party))))
	}
	lines = append(lines, fmt.Sprintf("%s gained %d XP and %d gold", c.Name, share, m.Def.Gold))
	lines = append(lines, h.grantXP(c, share)...)
	for _, ally := range party[1:] {
		lines = append(lines, fmt.Sprintf("%s gained %d XP (group share)", ally.Name, share))
		lines = append(lines, h.grantXP(ally, share)...)
		ally.send(fmt.Sprintf("EVT GROUP XP %s %s %d", c.Name, m.Def.ID, share))
	}
	if len(party) > 1 {
		h.log.Info("xp_shared", logging.Fields{
			"player": c.Name, "monster": m.Def.ID, "group": c.Group.ID, "players": len(party), "xp_each": share,
		})
	}

	for _, drop := range m.Def.Drops {
		if rand.Float64() > drop.Chance {
			continue
		}
		count := drop.Min + rand.IntN(drop.Max-drop.Min+1)
		for i := 0; i < count; i++ {
			h.newItem(drop.Item, "", c)
		}
		lines = append(lines, fmt.Sprintf("%s looted %d x %s", c.Name, count, drop.Item))
	}

	lines = append(lines, h.creditKill(c, m.Def.ID)...)
	return lines
}

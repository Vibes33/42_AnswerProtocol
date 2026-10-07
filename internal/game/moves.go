package game

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"

	"tap/internal/logging"
	"tap/internal/world"
)

const (
	errMoveNotFound   = "ERR 404 MOVE_NOT_FOUND"
	errMoveNotLearned = "ERR 403 MOVE_NOT_LEARNED"
	errNotEnoughMana  = "ERR 408 NOT_ENOUGH_MANA"
	errMoveNotOffered = "ERR 404 MOVE_NOT_OFFERED"
	errMoveSlotsFull  = "ERR 409 MOVE_SLOTS_FULL"
)

// moveSlots is how many attacks a character carries into combat (combat.max_equipped_moves,
// 3 by default). A new attack unlocked while every slot is taken waits in Offers until LEARN
// replaces an equipped one or skips it.
func (h *Hub) moveSlots() int {
	if n := h.world.Config.Combat.MaxEquippedMoves; n > 0 {
		return n
	}
	return 3
}

// buff is a temporary bonus (or penalty) on a stat, as a fraction: 0.5 = +50%.
type buff struct {
	Amount float64
	Turns  int
}

// splitAttack splits "ATTACK <monster> [<move>]". The move is an id ("move.rock_smash"): no
// monster name starts with "move.", so the last word cannot be ambiguous. Without a move,
// the RFC behaviour is kept: the basic attack.
func splitAttack(args string) (target, moveID string) {
	args = strings.TrimSpace(args)
	if i := strings.LastIndex(args, " "); i >= 0 && strings.HasPrefix(strings.ToLower(args[i+1:]), "move.") {
		return strings.TrimSpace(args[:i]), strings.ToLower(args[i+1:])
	}
	return args, ""
}

// chooseMove checks that the player knows the move and can pay its mana cost.
func (h *Hub) chooseMove(c *Client, moveID string) (*world.Move, string) {
	if moveID == "" {
		return h.world.Moves[basicAttackMove], ""
	}
	move := h.world.Moves[moveID]
	if move == nil {
		return nil, errMoveNotFound
	}
	if !slices.Contains(c.Moves, move.ID) {
		return nil, errMoveNotLearned
	}
	if c.Mana < h.manaCost(c, move) {
		return nil, errNotEnoughMana
	}
	return move, ""
}

// unlockMoves hands the character the attacks its level now allows: into a free slot, or
// as an offer when the three slots are taken. Each attack is unlocked only once, so a
// skipped or forgotten attack is never offered again. The basic attack comes first.
func (h *Hub) unlockMoves(c *Client) []string {
	var fresh []*world.Move
	for _, id := range sortedKeys(h.world.Moves) {
		if m := h.world.Moves[id]; !c.seen[id] && m.LearnableBy(c.Char, c.Level) {
			fresh = append(fresh, m)
		}
	}
	slices.SortStableFunc(fresh, func(a, b *world.Move) int {
		if a.Learn.Level != b.Learn.Level {
			return a.Learn.Level - b.Learn.Level
		}
		return boolRank(b.ID == basicAttackMove) - boolRank(a.ID == basicAttackMove)
	})
	var lines []string
	for _, m := range fresh {
		c.seen[m.ID] = true
		if len(c.Moves) < h.moveSlots() {
			c.Moves = append(c.Moves, m.ID)
			lines = append(lines, fmt.Sprintf("%s learned %s", c.Name, m.Name))
			continue
		}
		c.Offers = append(c.Offers, m.ID)
		lines = append(lines, fmt.Sprintf("%s can learn %s (LEARN to replace an attack or skip it)", c.Name, m.Name))
	}
	return lines
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// learn answers "LEARN <move> [<replaced-move>|skip]": takes an offered attack into a free
// slot, in place of an equipped one (forgotten for good), or turns the offer down.
func (h *Hub) learn(c *Client, args string) {
	words := strings.Fields(strings.ToLower(args))
	if len(words) == 0 {
		c.send(errMissingArgument)
		return
	}
	offer := slices.Index(c.Offers, words[0])
	if offer < 0 {
		c.send(errMoveNotOffered)
		return
	}
	moveID := words[0]
	switch {
	case len(words) > 1 && words[1] == "skip":
		c.Offers = slices.Delete(c.Offers, offer, offer+1)
		c.send("OK skipped=" + moveID)
	case len(words) > 1:
		slot := slices.Index(c.Moves, words[1])
		if slot < 0 {
			c.send(errMoveNotFound)
			return
		}
		c.Moves[slot] = moveID
		c.Offers = slices.Delete(c.Offers, offer, offer+1)
		c.send(fmt.Sprintf("OK learned=%s forgot=%s", moveID, words[1]))
	case len(c.Moves) < h.moveSlots():
		c.Moves = append(c.Moves, moveID)
		c.Offers = slices.Delete(c.Offers, offer, offer+1)
		c.send("OK learned=" + moveID)
	default:
		c.send(errMoveSlotsFull)
		return
	}
	h.log.Info("move_learn", logging.Fields{"player": c.Name, "args": args, "moves": c.Moves, "offers": c.Offers})
}

// manaCost applies the class multiplier (a fighter pays less than a mage).
func (h *Hub) manaCost(c *Client, m *world.Move) int {
	mult := h.world.Archetypes[c.Char.Archetype].ManaCostMultiplier
	if mult == 0 {
		mult = 1
	}
	return int(math.Ceil(float64(m.ManaCost) * mult))
}

// combatStats is the player's sheet in combat: its stats, changed by its active buffs.
func (h *Hub) combatStats(c *Client) world.Stats {
	s := h.statsOf(c)
	for stat, b := range c.Buffs {
		scale := func(v int) int { return max(1, int(float64(v)*(1+b.Amount))) }
		switch stat {
		case world.StatAttack:
			s.Attack = scale(s.Attack)
		case world.StatDefense:
			s.Defense = scale(s.Defense)
		case world.StatMagicAttack:
			s.MagicAttack = scale(s.MagicAttack)
		case world.StatMagicDefense:
			s.MagicDefense = scale(s.MagicDefense)
		case world.StatSpeed:
			s.Speed = scale(s.Speed)
		}
	}
	return s
}

// playerAct plays the player's turn with the chosen move: damage, or support (heal, buff…).
func (h *Hub) playerAct(c *Client, m *MonsterInstance, move *world.Move, stats world.Stats, lines []string) (int, []string) {
	if c.Statuses["status.sleep"] > 0 {
		return 0, append(lines, fmt.Sprintf("%s is asleep and cannot act", c.Name))
	}
	if move.Category == world.MoveStatus {
		lines = append(lines, fmt.Sprintf("%s used %s", c.Name, move.Name))
		for _, e := range move.Effects {
			if roll(e.Chance) {
				lines = h.supportEffect(c, m, move.Target, e, lines)
			}
		}
		return 0, lines
	}

	damage, note := h.computeDamage(move, c.Char.Type, stats, m.Def.Type, m.Def.Stats)
	if damage == 0 {
		return 0, append(lines, fmt.Sprintf("%s missed %s with %s", c.Name, m.Def.Name, move.Name))
	}
	m.HP -= damage
	lines = append(lines, fmt.Sprintf("%s hit %s with %s for %d%s", c.Name, m.Def.Name, move.Name, damage, note))
	for _, e := range move.Effects {
		if !roll(e.Chance) {
			continue
		}
		if e.Target == world.TargetSelf {
			lines = h.applyToPlayer(c, e, lines) // e.g. Siphon Strike gives mana back to the caster
		} else if e.Kind == world.EffectStatus {
			lines = h.applyEffects(&world.Move{Effects: []world.Effect{{Kind: e.Kind, Chance: 1, Status: e.Status, Duration: e.Duration}}}, nil, m, lines)
		}
	}
	return damage, lines
}

// supportEffect applies the effect of a support move. A group move (all_allies) also
// benefits the group members present in the room.
func (h *Hub) supportEffect(c *Client, m *MonsterInstance, target world.TargetKind, e world.Effect, lines []string) []string {
	if target == world.TargetEnemy || target == world.TargetAllEnemies {
		if e.Kind == world.EffectStatus {
			return h.applyEffects(&world.Move{Effects: []world.Effect{{Kind: e.Kind, Chance: 1, Status: e.Status, Duration: e.Duration}}}, nil, m, lines)
		}
		return lines
	}
	for _, ally := range h.alliesHere(c, target == world.TargetAllAllies) {
		lines = h.applyToPlayer(ally, e, lines)
	}
	return lines
}

// alliesHere returns the player, plus the members of its group in the same room when group is true.
func (h *Hub) alliesHere(c *Client, group bool) []*Client {
	allies := []*Client{c}
	if !group || c.Group == nil {
		return allies
	}
	for _, m := range c.Group.Members {
		if m != c && m.Room == c.Room && m.HP > 0 {
			allies = append(allies, m)
		}
	}
	return allies
}

// applyToPlayer applies a beneficial effect (or a deliberate penalty) to a player.
func (h *Hub) applyToPlayer(p *Client, e world.Effect, lines []string) []string {
	stats := h.statsOf(p)
	switch e.Kind {
	case world.EffectHeal, world.EffectHealPercent:
		amount := int(e.Amount)
		if e.Kind == world.EffectHealPercent {
			amount = int(float64(stats.HP) * e.Amount / 100)
		}
		healed := min(stats.HP, p.HP+amount) - p.HP
		p.HP += healed
		return append(lines, fmt.Sprintf("%s recovered %d HP", p.Name, healed))
	case world.EffectRestoreMana:
		gained := min(stats.Mana, p.Mana+int(e.Amount)) - p.Mana
		p.Mana += gained
		return append(lines, fmt.Sprintf("%s recovered %d mana", p.Name, gained))
	case world.EffectCure:
		if e.Status != "" {
			delete(p.Statuses, e.Status)
		} else {
			p.Statuses = map[string]int{}
		}
		return append(lines, fmt.Sprintf("%s is cured", p.Name))
	case world.EffectStatus:
		def := h.world.Statuses[e.Status]
		if def == nil {
			return lines
		}
		turns := e.Duration
		if turns == 0 {
			turns = def.DefaultDuration
		}
		p.Statuses[e.Status] = turns
		return append(lines, fmt.Sprintf("%s is %s", p.Name, def.Name))
	case world.EffectStatMod:
		p.Buffs[e.Stat] = &buff{Amount: e.Amount, Turns: max(1, e.Duration)}
		verb := "rose"
		if e.Amount < 0 {
			verb = "fell"
		}
		return append(lines, fmt.Sprintf("%s's %s %s", p.Name, strings.ReplaceAll(string(e.Stat), "_", " "), verb))
	}
	return lines
}

// tickBuffs expires buffs at the end of each combat round.
func tickBuffs(c *Client) {
	for stat, b := range c.Buffs {
		if b.Turns--; b.Turns <= 0 {
			delete(c.Buffs, stat)
		}
	}
}

// roll draws whether an effect with the given chance happens (1.0 = always).
func roll(chance float64) bool {
	return rand.Float64() <= chance
}

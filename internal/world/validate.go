package world

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type problems []error

func (p *problems) add(format string, args ...any) {
	*p = append(*p, fmt.Errorf(format, args...))
}

func has[T any](m map[string]T, id string) bool {
	_, ok := m[id]
	return ok
}

// sortedKeys gives a stable iteration order, so the same broken world always reports errors in the same order.
func sortedKeys[T any](m map[string]T) []string {
	return slices.Sorted(maps.Keys(m))
}

var opposite = map[string]string{
	"north": "south", "south": "north",
	"east": "west", "west": "east",
	"northeast": "southwest", "southwest": "northeast",
	"northwest": "southeast", "southeast": "northwest",
	"up": "down", "down": "up",
}

var (
	knownStats      = set(StatHP, StatMana, StatAttack, StatDefense, StatMagicAttack, StatMagicDefense, StatSpeed)
	knownTargets    = set(TargetEnemy, TargetAllEnemies, TargetSelf, TargetAlly, TargetAllAllies)
	knownCategories = set(ItemConsumable, ItemEquipment, ItemMaterial, ItemQuest, ItemKey, ItemFixture)
	knownRarities   = set(RarityCommon, RarityUncommon, RarityRare, RarityEpic, RarityLegendary)
	knownSlots      = set(SlotWeapon, SlotArmor, SlotAccessory)
	knownRoles      = set(RoleDialogue, RoleQuestGiver, RoleMerchant, RoleService)
	knownServices   = set(ServiceRestore, ServiceClassSelect)
)

func set[T comparable](values ...T) map[T]bool {
	m := make(map[T]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// Validate checks every cross-file reference and every subject requirement.
// It reports all problems at once instead of stopping at the first one.
func (w *World) Validate() error {
	var p problems
	w.validateConfig(&p)
	w.validateTypes(&p)
	w.validateCharacters(&p)
	w.validateMoves(&p)
	w.validateItems(&p)
	w.validateNPCs(&p)
	w.validateMonsters(&p)
	w.validateRooms(&p)
	w.validateQuests(&p)
	w.validateMap(&p)
	w.validateRequirements(&p)
	return errors.Join(p...)
}

func (w *World) hasType(e Element) bool {
	return slices.ContainsFunc(w.Types.Types, func(t TypeDef) bool { return t.ID == e })
}

func (w *World) isPlaced(npc string) bool {
	for _, r := range w.Rooms {
		if slices.Contains(r.NPCs, npc) {
			return true
		}
	}
	return false
}

func (w *World) isSpawned(monster string) bool {
	for _, r := range w.Rooms {
		for _, s := range r.Monsters {
			if s.Monster == monster {
				return true
			}
		}
	}
	return false
}

func (w *World) validateConfig(p *problems) {
	c := w.Config
	if !has(w.Rooms, c.StartRoom) {
		p.add("config: unknown start_room %q", c.StartRoom)
	}
	if r, ok := w.Rooms[c.RespawnRoom]; !ok {
		p.add("config: unknown respawn_room %q", c.RespawnRoom)
	} else if !r.Safe {
		p.add("config: respawn_room %q must be a safe room", c.RespawnRoom)
	}
	if !has(w.Characters, c.DefaultCharacter) {
		p.add("config: unknown default_character %q", c.DefaultCharacter)
	}
	if c.RespawnHPPercent < 1 || c.RespawnHPPercent > 100 {
		p.add("config: respawn_hp_percent must be between 1 and 100, got %d", c.RespawnHPPercent)
	}
	if c.MaxPlayers < 0 {
		p.add("config: max_players must be >= 0 (0 means unlimited), got %d", c.MaxPlayers)
	}

	if c.Leveling.MaxLevel < 1 {
		p.add("config: leveling.max_level must be >= 1")
	}
	if c.Leveling.XPBase < 1 || c.Leveling.XPPerLevel < 0 {
		p.add("config: leveling needs xp_base >= 1 and xp_per_level >= 0")
	}

	cb := c.Combat
	if cb.MaxEquippedMoves < 1 {
		p.add("config: combat.max_equipped_moves must be >= 1")
	}
	if cb.RestManaRegen < 0 {
		p.add("config: combat.rest_mana_regen must be >= 0")
	}
	if cb.STABMultiplier < 1 {
		p.add("config: combat.stab_multiplier must be >= 1")
	}
	if cb.CritChance < 0 || cb.CritChance > 1 {
		p.add("config: combat.crit_chance must be between 0 and 1")
	}
	if cb.CritMultiplier < 1 {
		p.add("config: combat.crit_multiplier must be >= 1")
	}

	if c.Inventory.BagCapacity < 1 {
		p.add("config: inventory.bag_capacity must be >= 1")
	}
	if len(c.Inventory.EquipmentSlots) == 0 {
		p.add("config: inventory.equipment_slots is empty")
	}
	seen := map[EquipSlot]bool{}
	for _, s := range c.Inventory.EquipmentSlots {
		if !knownSlots[s] {
			p.add("config: unknown equipment slot %q", s)
		}
		if seen[s] {
			p.add("config: equipment slot %q listed twice", s)
		}
		seen[s] = true
	}

	e := c.Economy
	if e.StartingGold < 0 {
		p.add("config: economy.starting_gold must be >= 0")
	}
	if e.SellRatio < 0 || e.SellRatio > 1 {
		p.add("config: economy.sell_ratio must be between 0 and 1")
	}
	if e.DeathGoldLossPercent < 0 || e.DeathGoldLossPercent > 100 {
		p.add("config: economy.death_gold_loss_percent must be between 0 and 100")
	}
}

func (w *World) validateTypes(p *problems) {
	seen := map[Element]bool{}
	for _, t := range w.Types.Types {
		if t.ID == "" {
			p.add("types: entry without id")
			continue
		}
		if seen[t.ID] {
			p.add("types: duplicate id %q", t.ID)
		}
		seen[t.ID] = true
	}
	for _, atk := range slices.Sorted(maps.Keys(w.Types.Effectiveness)) {
		if !seen[atk] {
			p.add("types: effectiveness uses unknown attacking type %q", atk)
		}
		row := w.Types.Effectiveness[atk]
		for _, def := range slices.Sorted(maps.Keys(row)) {
			if !seen[def] {
				p.add("types: effectiveness %s -> unknown defending type %q", atk, def)
			}
			if row[def] < 0 {
				p.add("types: effectiveness %s -> %s is negative", atk, def)
			}
		}
	}
}

func (w *World) validateCharacters(p *problems) {
	for _, id := range sortedKeys(w.Archetypes) {
		a := w.Archetypes[id]
		if a.BaseStats.HP != 100 {
			p.add("archetype %s: base hp must be 100 (subject: players start with 100 HP), got %d", id, a.BaseStats.HP)
		}
		if len(a.AllowedTypes) == 0 {
			p.add("archetype %s: allowed_types is empty", id)
		}
		for _, t := range a.AllowedTypes {
			if !w.hasType(t) {
				p.add("archetype %s: unknown type %q", id, t)
			}
		}
		if a.ManaCostMultiplier <= 0 {
			p.add("archetype %s: mana_cost_multiplier must be > 0", id)
		}
	}
	for _, id := range sortedKeys(w.Characters) {
		c := w.Characters[id]
		if a, ok := w.Archetypes[c.Archetype]; !ok {
			p.add("character %s: unknown archetype %q", id, c.Archetype)
		} else if !slices.Contains(a.AllowedTypes, c.Type) {
			p.add("character %s: type %q is not allowed for archetype %s", id, c.Type, c.Archetype)
		}
		w.validateStacks(p, "character "+id, c.StartingItems)
	}
}

func (w *World) validateMoves(p *problems) {
	for _, id := range sortedKeys(w.Moves) {
		m := w.Moves[id]
		owner := "move " + id
		if !w.hasType(m.Type) {
			p.add("%s: unknown type %q", owner, m.Type)
		}
		switch m.Category {
		case MovePhysical, MoveMagical:
			if m.Power <= 0 {
				p.add("%s: a %s move needs power > 0", owner, m.Category)
			}
		case MoveStatus:
			if m.Power != 0 {
				p.add("%s: a status move must have power 0", owner)
			}
			if len(m.Effects) == 0 {
				p.add("%s: a status move needs at least one effect", owner)
			}
		default:
			p.add("%s: unknown category %q", owner, m.Category)
		}
		if !knownTargets[m.Target] {
			p.add("%s: unknown target %q", owner, m.Target)
		}
		if m.Accuracy <= 0 || m.Accuracy > 1 {
			p.add("%s: accuracy must be in ]0, 1], got %v", owner, m.Accuracy)
		}
		if m.ManaCost < 0 {
			p.add("%s: mana_cost must be >= 0", owner)
		}
		w.validateEffects(p, owner, m.Effects)

		r := m.Learn
		if r == nil {
			continue
		}
		if r.Level < 1 {
			p.add("%s: learn.level must be >= 1", owner)
		}
		if len(r.Characters) > 0 && (len(r.Archetypes) > 0 || len(r.Types) > 0) {
			p.add("%s: a signature move (learn.characters) cannot also restrict archetypes or types", owner)
		}
		for _, a := range r.Archetypes {
			if !has(w.Archetypes, a) {
				p.add("%s: learn uses unknown archetype %q", owner, a)
			}
		}
		for _, t := range r.Types {
			if !w.hasType(t) {
				p.add("%s: learn uses unknown type %q", owner, t)
			}
		}
		for _, c := range r.Characters {
			if !has(w.Characters, c) {
				p.add("%s: learn uses unknown character %q", owner, c)
			}
		}
	}
}

func (w *World) validateEffects(p *problems, owner string, effects []Effect) {
	for i, e := range effects {
		where := fmt.Sprintf("%s: effect #%d (%s)", owner, i+1, e.Kind)
		if e.Chance <= 0 || e.Chance > 1 {
			p.add("%s: chance must be in ]0, 1], got %v", where, e.Chance)
		}
		if e.Target != "" && !knownTargets[e.Target] {
			p.add("%s: unknown target %q", where, e.Target)
		}
		switch e.Kind {
		case EffectStatus:
			if !has(w.Statuses, e.Status) {
				p.add("%s: unknown status %q", where, e.Status)
			}
		case EffectStatMod:
			if !knownStats[e.Stat] {
				p.add("%s: unknown stat %q", where, e.Stat)
			}
			if e.Amount == 0 {
				p.add("%s: amount must not be 0", where)
			}
			if e.Duration < 1 {
				p.add("%s: duration must be >= 1", where)
			}
		case EffectCure:
			if e.Status != "" && !has(w.Statuses, e.Status) {
				p.add("%s: unknown status %q", where, e.Status)
			}
		case EffectHeal, EffectRestoreMana:
			if e.Amount <= 0 {
				p.add("%s: amount must be > 0", where)
			}
		case EffectHealPercent:
			if e.Amount <= 0 || e.Amount > 100 {
				p.add("%s: amount must be in ]0, 100]", where)
			}
		default:
			p.add("%s: unknown effect kind", where)
		}
	}
}

func (w *World) validateStacks(p *problems, owner string, stacks []ItemStack) {
	for _, s := range stacks {
		if !has(w.Items, s.Item) {
			p.add("%s: unknown item %q", owner, s.Item)
		}
		if s.Count < 1 {
			p.add("%s: item %q count must be >= 1", owner, s.Item)
		}
	}
}

func (w *World) validateItems(p *problems) {
	for _, id := range sortedKeys(w.Items) {
		it := w.Items[id]
		owner := "item " + id
		if !knownCategories[it.Category] {
			p.add("%s: unknown category %q", owner, it.Category)
		}
		if !knownRarities[it.Rarity] {
			p.add("%s: unknown rarity %q", owner, it.Rarity)
		}
		if it.Price < 0 {
			p.add("%s: price must be >= 0", owner)
		}
		if it.LevelRequired < 0 {
			p.add("%s: level_required must be >= 0", owner)
		}
		if (it.Category == ItemEquipment) != (it.Equipment != nil) {
			p.add("%s: category \"equipment\" and the equipment block must be used together", owner)
		}
		if it.Category == ItemConsumable && len(it.Effects) == 0 {
			p.add("%s: a consumable needs at least one effect", owner)
		}
		if it.UsableInCombat && len(it.Effects) == 0 {
			p.add("%s: usable_in_combat but has no effect", owner)
		}
		if it.Category == ItemFixture && it.Obtainable {
			p.add("%s: a fixture cannot be obtainable", owner)
		}
		w.validateEffects(p, owner, it.Effects)

		if eq := it.Equipment; eq != nil {
			if !slices.Contains(w.Config.Inventory.EquipmentSlots, eq.Slot) {
				p.add("%s: slot %q is not in config equipment_slots", owner, eq.Slot)
			}
			for _, a := range eq.Archetypes {
				if !has(w.Archetypes, a) {
					p.add("%s: unknown archetype %q", owner, a)
				}
			}
		}
	}
}

func (w *World) validateNPCs(p *problems) {
	for _, id := range sortedKeys(w.NPCs) {
		n := w.NPCs[id]
		owner := "npc " + id
		if len(n.Roles) == 0 {
			p.add("%s: has no role", owner)
		}
		for _, r := range n.Roles {
			if !knownRoles[r] {
				p.add("%s: unknown role %q", owner, r)
			}
		}
		if n.HasRole(RoleDialogue) && len(n.Dialogue) == 0 {
			p.add("%s: role dialogue but no dialogue lines", owner)
		}
		if n.HasRole(RoleQuestGiver) != (len(n.Quests) > 0) {
			p.add("%s: role quest_giver and the quests list must be used together", owner)
		}
		if n.HasRole(RoleMerchant) != (len(n.Shop) > 0) {
			p.add("%s: role merchant and the shop list must be used together", owner)
		}
		if n.HasRole(RoleService) != (n.Service != nil) {
			p.add("%s: role service and the service block must be used together", owner)
		}

		for _, q := range n.Quests {
			if quest, ok := w.Quests[q]; !ok {
				p.add("%s: unknown quest %q", owner, q)
			} else if quest.Giver != id {
				p.add("%s: lists quest %s, but its giver is %s", owner, q, quest.Giver)
			}
		}
		for _, s := range n.Shop {
			if it, ok := w.Items[s.Item]; !ok {
				p.add("%s: shop sells unknown item %q", owner, s.Item)
			} else if BuyPrice(s, it) <= 0 {
				p.add("%s: shop sells %q without a price", owner, s.Item)
			}
		}
		if sv := n.Service; sv != nil {
			if !knownServices[sv.Kind] {
				p.add("%s: unknown service %q", owner, sv.Kind)
			}
			if sv.Price < 0 {
				p.add("%s: service price must be >= 0", owner)
			}
		}
	}
}

func (w *World) validateMonsters(p *problems) {
	for _, id := range sortedKeys(w.Monsters) {
		m := w.Monsters[id]
		owner := "monster " + id
		if !w.hasType(m.Type) {
			p.add("%s: unknown type %q", owner, m.Type)
		}
		if m.Level < 1 {
			p.add("%s: level must be >= 1", owner)
		}
		if m.Stats.HP < 1 {
			p.add("%s: hp must be >= 1", owner)
		}
		if m.XP < 1 {
			p.add("%s: must give xp (>= 1)", owner)
		}
		if m.Gold < 0 {
			p.add("%s: gold must be >= 0", owner)
		}
		if len(m.Moves) == 0 {
			p.add("%s: has no moves", owner)
		}
		for _, mv := range m.Moves {
			if !has(w.Moves, mv) {
				p.add("%s: unknown move %q", owner, mv)
			}
		}
		for _, d := range m.Drops {
			if !has(w.Items, d.Item) {
				p.add("%s: drops unknown item %q", owner, d.Item)
			}
			if d.Chance <= 0 || d.Chance > 1 {
				p.add("%s: drop %q chance must be in ]0, 1]", owner, d.Item)
			}
			if d.Min < 1 || d.Max < d.Min {
				p.add("%s: drop %q needs 1 <= min <= max", owner, d.Item)
			}
		}
	}
}

func (w *World) validateRooms(p *problems) {
	placedIn := map[string]string{}
	for _, id := range sortedKeys(w.Rooms) {
		r := w.Rooms[id]
		owner := "room " + id
		if !has(w.Zones, r.Zone) {
			p.add("%s: unknown zone %q", owner, r.Zone)
		}
		if len(r.Exits) == 0 {
			p.add("%s: has no exits", owner)
		}
		for _, dir := range sortedKeys(r.Exits) {
			dest := r.Exits[dir]
			back, knownDir := opposite[dir]
			if !knownDir {
				p.add("%s: unknown direction %q", owner, dir)
			}
			if dest == id {
				p.add("%s: exit %s leads to itself", owner, dir)
				continue
			}
			d, ok := w.Rooms[dest]
			if !ok {
				p.add("%s: exit %s leads to unknown room %q", owner, dir, dest)
				continue
			}
			if knownDir && d.Exits[back] != id {
				p.add("%s: exit %s to %s has no way back (%s needs \"%s\": %q)", owner, dir, dest, dest, back, id)
			}
		}

		w.validateStacks(p, owner, r.Items)

		for _, n := range r.NPCs {
			if !has(w.NPCs, n) {
				p.add("%s: unknown npc %q", owner, n)
				continue
			}
			if prev, dup := placedIn[n]; dup {
				p.add("%s: npc %s is already placed in %s (an NPC is unique)", owner, n, prev)
			}
			placedIn[n] = id
		}
		for _, s := range r.Monsters {
			if !has(w.Monsters, s.Monster) {
				p.add("%s: unknown monster %q", owner, s.Monster)
			}
			if s.Count < 1 {
				p.add("%s: monster %q count must be >= 1", owner, s.Monster)
			}
			if s.RespawnSeconds < 0 {
				p.add("%s: monster %q respawn_seconds must be >= 0", owner, s.Monster)
			}
			if r.Safe {
				p.add("%s: monster %q in a safe room", owner, s.Monster)
			}
		}
	}
}

func (w *World) validateQuests(p *problems) {
	for _, id := range sortedKeys(w.Quests) {
		q := w.Quests[id]
		owner := "quest " + id
		if giver, ok := w.NPCs[q.Giver]; !ok {
			p.add("%s: unknown giver %q", owner, q.Giver)
		} else {
			if !slices.Contains(giver.Quests, id) {
				p.add("%s: giver %s does not list this quest", owner, q.Giver)
			}
			if !w.isPlaced(q.Giver) {
				p.add("%s: giver %s is not placed in any room", owner, q.Giver)
			}
		}
		if q.LevelRequired < 0 {
			p.add("%s: level_required must be >= 0", owner)
		}
		for _, pre := range q.Prerequisites {
			if pre == id {
				p.add("%s: requires itself", owner)
			} else if !has(w.Quests, pre) {
				p.add("%s: unknown prerequisite %q", owner, pre)
			}
		}

		obj := q.Objective
		if obj.Count < 1 {
			p.add("%s: objective count must be >= 1", owner)
		}
		switch obj.Kind {
		case ObjectiveFetch, ObjectiveDeliver:
			if it, ok := w.Items[obj.Target]; !ok {
				p.add("%s: unknown target item %q", owner, obj.Target)
			} else if !it.Obtainable {
				p.add("%s: target item %q can never be obtained", owner, obj.Target)
			}
		case ObjectiveDefeat:
			if !has(w.Monsters, obj.Target) {
				p.add("%s: unknown target monster %q", owner, obj.Target)
			} else if !w.isSpawned(obj.Target) {
				p.add("%s: target monster %q never spawns", owner, obj.Target)
			}
		default:
			p.add("%s: unknown objective kind %q", owner, obj.Kind)
		}
		if obj.TurnIn != "" {
			if !has(w.NPCs, obj.TurnIn) {
				p.add("%s: unknown turn_in npc %q", owner, obj.TurnIn)
			} else if !w.isPlaced(obj.TurnIn) {
				p.add("%s: turn_in npc %s is not placed in any room", owner, obj.TurnIn)
			}
		}

		if q.Reward.XP < 0 || q.Reward.Gold < 0 {
			p.add("%s: reward xp and gold must be >= 0", owner)
		}
		w.validateStacks(p, owner+" reward", q.Reward.Items)
	}
	w.validateQuestChains(p)
}

// validateQuestChains rejects prerequisite cycles (A needs B, B needs A): such quests could never start.
func (w *World) validateQuestChains(p *problems) {
	const (
		unvisited = iota
		visiting
		done
	)
	state := map[string]int{}
	var inCycle func(id string) bool
	inCycle = func(id string) bool {
		switch state[id] {
		case visiting:
			return true
		case done:
			return false
		}
		state[id] = visiting
		if q, ok := w.Quests[id]; ok {
			for _, pre := range q.Prerequisites {
				if pre != id && inCycle(pre) {
					return true
				}
			}
		}
		state[id] = done
		return false
	}
	for _, id := range sortedKeys(w.Quests) {
		if state[id] == unvisited && inCycle(id) {
			p.add("quest %s: prerequisites form or lead into a cycle", id)
		}
	}
}

// validateMap walks the map from the start room. Exits are two-way, so the map is an
// undirected graph: a connected graph with at least as many passages as rooms contains a loop.
func (w *World) validateMap(p *problems) {
	start := w.Config.StartRoom
	if !has(w.Rooms, start) {
		return
	}
	seen := map[string]bool{start: true}
	passages := map[[2]string]bool{}
	queue := []string{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, dest := range w.Rooms[id].Exits {
			if dest == id || !has(w.Rooms, dest) {
				continue
			}
			passages[[2]string{min(id, dest), max(id, dest)}] = true
			if !seen[dest] {
				seen[dest] = true
				queue = append(queue, dest)
			}
		}
	}

	var unreachable []string
	for _, id := range sortedKeys(w.Rooms) {
		if !seen[id] {
			unreachable = append(unreachable, id)
		}
	}
	if len(unreachable) > 0 {
		p.add("map: rooms unreachable from %s: %s", start, strings.Join(unreachable, ", "))
	}
	if len(passages) < len(seen) {
		p.add("map: no loop found (subject: movement must allow a full circuit)")
	}
}

// validateRequirements enforces the minimum world size demanded by the subject.
func (w *World) validateRequirements(p *problems) {
	if len(w.Rooms) < 8 {
		p.add("subject: at least 8 rooms required, found %d", len(w.Rooms))
	}

	var dialogue, questGiver bool
	for _, n := range w.NPCs {
		dialogue = dialogue || n.HasRole(RoleDialogue)
		questGiver = questGiver || n.HasRole(RoleQuestGiver)
	}
	if !dialogue || !questGiver || len(w.Monsters) == 0 {
		p.add("subject: 3 NPC roles required (dialogue npc, quest-giver npc, enemy monster)")
	}

	obtainable := 0
	for _, it := range w.Items {
		if it.Obtainable {
			obtainable++
		}
	}
	if len(w.Items) < 4 || obtainable < 2 {
		p.add("subject: at least 4 items with 2 obtainable required, found %d with %d obtainable", len(w.Items), obtainable)
	}
	if len(w.Quests) < 2 {
		p.add("subject: at least 2 quests required, found %d", len(w.Quests))
	}
}

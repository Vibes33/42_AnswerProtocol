package world

import (
	"errors"
	"fmt"
	"maps"
	"slices"
)

type problems []error

func (p *problems) add(format string, args ...any) {
	*p = append(*p, fmt.Errorf(format, args...))
}

func has[T any](m map[string]T, id string) bool {
	_, ok := m[id]
	return ok
}

// sortedKeys gives a stable iteration order, so the same broken world always reports
// its errors in the same order.
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

// Validate checks that every id referenced by the world data exists, and reports
// all the problems it finds at once instead of stopping at the first one.
func (w *World) Validate() error {
	var p problems
	w.checkConfig(&p)
	w.checkCharacters(&p)
	w.checkMoves(&p)
	w.checkItems(&p)
	w.checkNPCs(&p)
	w.checkMonsters(&p)
	w.checkRooms(&p)
	w.checkQuests(&p)
	return errors.Join(p...)
}

func (w *World) hasType(e Element) bool {
	return slices.ContainsFunc(w.Types.Types, func(t TypeDef) bool { return t.ID == e })
}

func (w *World) checkConfig(p *problems) {
	c := w.Config
	if !has(w.Rooms, c.StartRoom) {
		p.add("config: unknown start_room %q", c.StartRoom)
	}
	if room, ok := w.Rooms[c.RespawnRoom]; !ok {
		p.add("config: unknown respawn_room %q", c.RespawnRoom)
	} else if !room.Safe {
		p.add("config: respawn_room %q must be a safe room", c.RespawnRoom)
	}
	if !has(w.Characters, c.DefaultCharacter) {
		p.add("config: unknown default_character %q", c.DefaultCharacter)
	}
}

func (w *World) checkCharacters(p *problems) {
	for _, id := range sortedKeys(w.Archetypes) {
		a := w.Archetypes[id]
		// The subject requires every player to start with 100 health points.
		if a.BaseStats.HP != 100 {
			p.add("archetype %s: base hp must be 100, got %d", id, a.BaseStats.HP)
		}
		for _, t := range a.AllowedTypes {
			if !w.hasType(t) {
				p.add("archetype %s: unknown type %q", id, t)
			}
		}
	}
	for _, id := range sortedKeys(w.Characters) {
		c := w.Characters[id]
		a, ok := w.Archetypes[c.Archetype]
		if !ok {
			p.add("character %s: unknown archetype %q", id, c.Archetype)
			continue
		}
		if !slices.Contains(a.AllowedTypes, c.Type) {
			p.add("character %s: type %q is not allowed for archetype %s", id, c.Type, c.Archetype)
		}
		w.checkItemRefs(p, "character "+id, c.StartingItems)
	}
}

func (w *World) checkMoves(p *problems) {
	for _, id := range sortedKeys(w.Moves) {
		m := w.Moves[id]
		if !w.hasType(m.Type) {
			p.add("move %s: unknown type %q", id, m.Type)
		}
		w.checkEffects(p, "move "+id, m.Effects)
	}
}

// checkEffects only verifies the ids an effect points at; the combat engine decides
// what each effect does.
func (w *World) checkEffects(p *problems, owner string, effects []Effect) {
	for _, e := range effects {
		if e.Status != "" && !has(w.Statuses, e.Status) {
			p.add("%s: unknown status %q", owner, e.Status)
		}
	}
}

func (w *World) checkItemRefs(p *problems, owner string, stacks []ItemStack) {
	for _, s := range stacks {
		if !has(w.Items, s.Item) {
			p.add("%s: unknown item %q", owner, s.Item)
		}
	}
}

func (w *World) checkItems(p *problems) {
	for _, id := range sortedKeys(w.Items) {
		it := w.Items[id]
		w.checkEffects(p, "item "+id, it.Effects)
		if (it.Category == ItemEquipment) != (it.Equipment != nil) {
			p.add("item %s: category \"equipment\" and the equipment block must be used together", id)
		}
		if it.Equipment == nil {
			continue
		}
		for _, a := range it.Equipment.Archetypes {
			if !has(w.Archetypes, a) {
				p.add("item %s: unknown archetype %q", id, a)
			}
		}
	}
}

func (w *World) checkNPCs(p *problems) {
	for _, id := range sortedKeys(w.NPCs) {
		n := w.NPCs[id]
		for _, q := range n.Quests {
			if quest, ok := w.Quests[q]; !ok {
				p.add("npc %s: unknown quest %q", id, q)
			} else if quest.Giver != id {
				p.add("npc %s: lists quest %s, but its giver is %s", id, q, quest.Giver)
			}
		}
		for _, s := range n.Shop {
			if !has(w.Items, s.Item) {
				p.add("npc %s: shop sells unknown item %q", id, s.Item)
			}
		}
	}
}

func (w *World) checkMonsters(p *problems) {
	for _, id := range sortedKeys(w.Monsters) {
		m := w.Monsters[id]
		if !w.hasType(m.Type) {
			p.add("monster %s: unknown type %q", id, m.Type)
		}
		if m.Stats.HP < 1 {
			p.add("monster %s: hp must be >= 1", id)
		}
		if len(m.Moves) == 0 {
			p.add("monster %s: has no moves", id)
		}
		for _, mv := range m.Moves {
			if !has(w.Moves, mv) {
				p.add("monster %s: unknown move %q", id, mv)
			}
		}
		for _, d := range m.Drops {
			if !has(w.Items, d.Item) {
				p.add("monster %s: drops unknown item %q", id, d.Item)
			}
		}
	}
}

func (w *World) checkRooms(p *problems) {
	for _, id := range sortedKeys(w.Rooms) {
		r := w.Rooms[id]
		if !has(w.Zones, r.Zone) {
			p.add("room %s: unknown zone %q", id, r.Zone)
		}
		for _, dir := range sortedKeys(r.Exits) {
			dest := r.Exits[dir]
			back, known := opposite[dir]
			if !known {
				p.add("room %s: unknown direction %q", id, dir)
			}
			if !has(w.Rooms, dest) {
				p.add("room %s: exit %s leads to unknown room %q", id, dir, dest)
				continue
			}
			// Every exit must be walkable both ways, otherwise a player can get stuck.
			if known && w.Rooms[dest].Exits[back] != id {
				p.add("room %s: exit %s to %s has no way back (%s needs \"%s\": %q)", id, dir, dest, dest, back, id)
			}
		}
		w.checkItemRefs(p, "room "+id, r.Items)
		for _, n := range r.NPCs {
			if !has(w.NPCs, n) {
				p.add("room %s: unknown npc %q", id, n)
			}
		}
		for _, s := range r.Monsters {
			if !has(w.Monsters, s.Monster) {
				p.add("room %s: unknown monster %q", id, s.Monster)
			}
			if r.Safe {
				p.add("room %s: monster %q in a safe room", id, s.Monster)
			}
		}
	}
}

func (w *World) checkQuests(p *problems) {
	for _, id := range sortedKeys(w.Quests) {
		q := w.Quests[id]
		if !has(w.NPCs, q.Giver) {
			p.add("quest %s: unknown giver %q", id, q.Giver)
		}
		if q.Objective.TurnIn != "" && !has(w.NPCs, q.Objective.TurnIn) {
			p.add("quest %s: unknown turn_in npc %q", id, q.Objective.TurnIn)
		}
		for _, pre := range q.Prerequisites {
			if !has(w.Quests, pre) {
				p.add("quest %s: unknown prerequisite %q", id, pre)
			}
		}
		switch q.Objective.Kind {
		case ObjectiveFetch, ObjectiveDeliver:
			if !has(w.Items, q.Objective.Target) {
				p.add("quest %s: unknown target item %q", id, q.Objective.Target)
			}
		case ObjectiveDefeat:
			if !has(w.Monsters, q.Objective.Target) {
				p.add("quest %s: unknown target monster %q", id, q.Objective.Target)
			}
		default:
			p.add("quest %s: unknown objective kind %q", id, q.Objective.Kind)
		}
		w.checkItemRefs(p, "quest "+id+" reward", q.Reward.Items)
	}
}

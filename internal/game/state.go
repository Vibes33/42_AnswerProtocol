package game

import (
	"fmt"
	"sort"
	"strings"

	"tap/internal/world"
)

// ItemInstance is one copy of an item definition. Room is empty while the item is carried.
type ItemInstance struct {
	ID     string
	Def    *world.Item
	Room   string
	Holder *Client
}

// MonsterInstance is one spawned creature with its own health.
type MonsterInstance struct {
	ID   string
	Def  *world.Monster
	Room string
	HP   int
}

// spawn fills the world with its initial items and monsters. Instance ids stay internal:
// the protocol exposes definition ids, so two identical items in a room are listed twice.
func (h *Hub) spawn() {
	counter := map[string]int{}
	nextID := func(def string) string {
		counter[def]++
		return fmt.Sprintf("%s#%d", def, counter[def])
	}

	for _, roomID := range sortedKeys(h.world.Rooms) {
		room := h.world.Rooms[roomID]
		for _, stack := range room.Items {
			for i := 0; i < stack.Count; i++ {
				id := nextID(stack.Item)
				h.items[id] = &ItemInstance{ID: id, Def: h.world.Items[stack.Item], Room: roomID}
			}
		}
		for _, s := range room.Monsters {
			for i := 0; i < s.Count; i++ {
				def := h.world.Monsters[s.Monster]
				id := nextID(s.Monster)
				h.monsters[id] = &MonsterInstance{ID: id, Def: def, Room: roomID, HP: def.Stats.HP}
			}
		}
	}
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// normalizeQuery prepares what a player typed for comparison: "  Healing Herb " -> "healing herb".
func normalizeQuery(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// itemMatches accepts the canonical id ("item.healing_herb"), that id without its prefix
// ("healing_herb") or the display name ("Healing Herb"), spaces and case included.
func itemMatches(def *world.Item, query string) bool {
	if query == "" {
		return false
	}
	id := strings.ToLower(def.ID)
	return query == id ||
		query == strings.ToLower(def.Name) ||
		"item."+query == id
}

// findItem returns the first matching instance among those the filter accepts.
// Instances are visited in id order, so the same query always picks the same copy.
func (h *Hub) findItem(query string, accept func(*ItemInstance) bool) *ItemInstance {
	q := normalizeQuery(query)
	for _, id := range sortedKeys(h.items) {
		inst := h.items[id]
		if accept(inst) && itemMatches(inst.Def, q) {
			return inst
		}
	}
	return nil
}

// inventoryOf lists what a player carries, in id order.
func (h *Hub) inventoryOf(c *Client) []*ItemInstance {
	var held []*ItemInstance
	for _, id := range sortedKeys(h.items) {
		if h.items[id].Holder == c {
			held = append(held, h.items[id])
		}
	}
	return held
}

// dropAll puts everything a leaving player carried back on the floor, so items are never
// lost with a disconnection.
func (h *Hub) dropAll(c *Client, room string) {
	for _, inst := range h.inventoryOf(c) {
		inst.Holder = nil
		inst.Room = room
	}
}

// playersIn lists the authenticated players standing in a room.
func (h *Hub) playersIn(room string) []string {
	names := []string{}
	for c := range h.clients {
		if c.authenticated() && c.Room == room {
			names = append(names, c.Name)
		}
	}
	sort.Strings(names)
	return names
}

// itemsIn lists the definition ids of the items lying in a room.
func (h *Hub) itemsIn(room string) []string {
	ids := []string{}
	for _, inst := range h.items {
		if inst.Room == room {
			ids = append(ids, inst.Def.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

// npcsIn lists the definition ids of every character in a room, peaceful NPCs and monsters alike:
// RFC 42TAP has a single "npcs" field.
func (h *Hub) npcsIn(room string) []string {
	ids := append([]string{}, h.world.Rooms[room].NPCs...)
	for _, m := range h.monsters {
		if m.Room == room {
			ids = append(ids, m.Def.ID)
		}
	}
	sort.Strings(ids)
	return ids
}

package game

import (
	"slices"

	"tap/internal/logging"
	"tap/internal/world"
)

// USE (extension): consume an item from the bag, or equip / unequip a piece of equipment.
const (
	errItemNotUsable = "ERR 409 ITEM_NOT_USABLE"
	errLevelTooLow   = "ERR 403 LEVEL_TOO_LOW"
	errWrongClass    = "ERR 403 WRONG_CLASS"
)

// useReply tells the client what happened and the player's new health and mana.
type useReply struct {
	Item    string   `json:"item"`
	Action  string   `json:"action"` // "used", "equipped" or "unequipped"
	HP      int      `json:"hp"`
	MaxHP   int      `json:"max_hp"`
	Mana    int      `json:"mana"`
	MaxMana int      `json:"max_mana"`
	Log     []string `json:"log,omitempty"`
}

// bagEntry describes one kind of item in the player's bag, for STATUS.
type bagEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Count       int    `json:"count"`
	Equipped    bool   `json:"equipped,omitempty"`
	Usable      bool   `json:"usable"`
}

// usable reports whether USE does something with the item: apply its effects, or equip it.
func usable(it *world.Item) bool {
	return it.Equipment != nil || len(it.Effects) > 0
}

// equipped returns the equipment the player currently wears, per slot. An item that left the
// bag (dropped, sold, handed in for a quest) is no longer worn: it is pruned here.
func (h *Hub) equipped(c *Client) map[world.EquipSlot]*ItemInstance {
	worn := map[world.EquipSlot]*ItemInstance{}
	for slot, id := range c.Equipped {
		inst := h.items[id]
		if inst == nil || inst.Holder != c {
			delete(c.Equipped, slot)
			continue
		}
		worn[slot] = inst
	}
	return worn
}

// use answers USE <item>.
func (h *Hub) use(c *Client, query string) {
	if normalizeQuery(query) == "" {
		c.send(errMissingArgument)
		return
	}
	inst := h.findItem(query, func(i *ItemInstance) bool { return i.Holder == c })
	if inst == nil {
		c.send(errItemNotInInventory)
		return
	}
	it := inst.Def
	reply := useReply{Item: it.ID}

	switch {
	case it.Equipment != nil:
		slot := it.Equipment.Slot
		if worn := h.equipped(c)[slot]; worn != nil && worn.Def.ID == it.ID {
			delete(c.Equipped, slot) // using worn equipment again takes it off
			reply.Action = "unequipped"
			break
		}
		if c.Level < it.LevelRequired {
			c.send(errLevelTooLow)
			return
		}
		if len(it.Equipment.Archetypes) > 0 && !slices.Contains(it.Equipment.Archetypes, c.Char.Archetype) {
			c.send(errWrongClass)
			return
		}
		c.Equipped[slot] = inst.ID
		reply.Action = "equipped"
	case len(it.Effects) > 0:
		for _, e := range it.Effects {
			if roll(e.Chance) {
				reply.Log = h.applyToPlayer(c, e, reply.Log)
			}
		}
		delete(h.items, inst.ID) // a consumable is used up
		reply.Action = "used"
	default:
		c.send(errItemNotUsable)
		return
	}

	// Taking off equipment can lower the maximums below the current values.
	stats := h.statsOf(c)
	c.HP, c.Mana = min(c.HP, stats.HP), min(c.Mana, stats.Mana)
	reply.HP, reply.MaxHP, reply.Mana, reply.MaxMana = c.HP, stats.HP, c.Mana, stats.Mana

	h.log.Info("item_"+reply.Action, logging.Fields{"player": c.Name, "item": it.ID, "hp": c.HP, "mana": c.Mana})
	h.sendJSON(c, reply)
}

// bagOf summarises the bag for STATUS: one entry per kind of item, in inventory order.
func (h *Hub) bagOf(c *Client) []bagEntry {
	worn := map[string]bool{}
	for _, inst := range h.equipped(c) {
		worn[inst.Def.ID] = true
	}
	bag := []bagEntry{}
	index := map[string]int{}
	for _, inst := range h.inventoryOf(c) {
		if i, ok := index[inst.Def.ID]; ok {
			bag[i].Count++
			continue
		}
		index[inst.Def.ID] = len(bag)
		bag = append(bag, bagEntry{
			ID: inst.Def.ID, Name: inst.Def.Name, Description: inst.Def.Description,
			Category: string(inst.Def.Category), Count: 1, Equipped: worn[inst.Def.ID], Usable: usable(inst.Def),
		})
	}
	return bag
}

// equipmentOf lists the worn equipment for STATUS, as slot -> item id.
func (h *Hub) equipmentOf(c *Client) map[string]string {
	out := map[string]string{}
	for slot, inst := range h.equipped(c) {
		out[string(slot)] = inst.Def.ID
	}
	return out
}

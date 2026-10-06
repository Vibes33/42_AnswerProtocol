package game

import (
	"fmt"

	"tap/internal/logging"
	"tap/internal/world"
)

// Trading (extension): SHOP, BUY and SELL with the merchant present in the room.
const (
	errNoMerchant      = "ERR 404 NO_MERCHANT"
	errNotSold         = "ERR 404 ITEM_NOT_SOLD"
	errNotEnoughGold   = "ERR 407 NOT_ENOUGH_GOLD"
	errItemNotSellable = "ERR 409 ITEM_NOT_SELLABLE"
)

type shopItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Category      string `json:"category"`
	Price         int    `json:"price"`
	LevelRequired int    `json:"level_required,omitempty"`
	Count         int    `json:"count,omitempty"` // for resale: how many the player carries
}

type shopReply struct {
	Merchant string     `json:"merchant"`
	Name     string     `json:"name"`
	Gold     int        `json:"gold"`
	Wares    []shopItem `json:"wares"`
	Buyback  []shopItem `json:"buyback"` // what the merchant buys back from the player, and at what price
}

// merchantHere returns the merchant of the player's room, if there is one.
func (h *Hub) merchantHere(c *Client) *world.NPC {
	for _, id := range h.npcsIn(c.Room) {
		if n := h.world.NPCs[id]; n != nil && n.HasRole(world.RoleMerchant) {
			return n
		}
	}
	return nil
}

func priceOf(e world.ShopEntry, it *world.Item) int {
	if e.Price > 0 {
		return e.Price
	}
	return it.Price
}

// shop answers SHOP: the wares for sale and the resale price of what the player carries.
func (h *Hub) shop(c *Client) {
	n := h.merchantHere(c)
	if n == nil {
		c.send(errNoMerchant)
		return
	}
	reply := shopReply{Merchant: n.ID, Name: n.Name, Gold: c.Gold, Wares: []shopItem{}, Buyback: []shopItem{}}
	for _, e := range n.Shop {
		it := h.world.Items[e.Item]
		reply.Wares = append(reply.Wares, shopItem{
			ID: it.ID, Name: it.Name, Description: it.Description, Category: string(it.Category),
			Price: priceOf(e, it), LevelRequired: it.LevelRequired,
		})
	}
	counts := map[string]int{}
	var order []string
	for _, inst := range h.inventoryOf(c) {
		if counts[inst.Def.ID] == 0 {
			order = append(order, inst.Def.ID)
		}
		counts[inst.Def.ID]++
	}
	for _, id := range order {
		it := h.world.Items[id]
		if price := h.world.Config.Economy.SellPrice(it); price > 0 {
			reply.Buyback = append(reply.Buyback, shopItem{
				ID: it.ID, Name: it.Name, Description: it.Description, Category: string(it.Category),
				Price: price, Count: counts[id],
			})
		}
	}
	h.sendJSON(c, reply)
}

// buy answers BUY <item>: the item (id or name) must be sold by the merchant of the room.
func (h *Hub) buy(c *Client, query string) {
	n := h.merchantHere(c)
	if n == nil {
		c.send(errNoMerchant)
		return
	}
	q := normalizeQuery(query)
	if q == "" {
		c.send(errMissingArgument)
		return
	}
	for _, e := range n.Shop {
		it := h.world.Items[e.Item]
		if !itemMatches(it, q) {
			continue
		}
		price := priceOf(e, it)
		if c.Gold < price {
			c.send(errNotEnoughGold)
			return
		}
		if len(h.inventoryOf(c)) >= h.world.Config.Inventory.BagCapacity {
			c.send(errInventoryFull)
			return
		}
		c.Gold -= price
		h.newItem(it.ID, "", c)
		h.log.Info("item_bought", logging.Fields{"player": c.Name, "merchant": n.ID, "item": it.ID, "price": price, "gold": c.Gold})
		c.send(fmt.Sprintf("OK bought=%s gold=%d", it.ID, c.Gold))
		return
	}
	c.send(errNotSold)
}

// sell answers SELL <item>: the merchant of the room buys an item from the bag.
func (h *Hub) sell(c *Client, query string) {
	n := h.merchantHere(c)
	if n == nil {
		c.send(errNoMerchant)
		return
	}
	if normalizeQuery(query) == "" {
		c.send(errMissingArgument)
		return
	}
	inst := h.findItem(query, func(i *ItemInstance) bool { return i.Holder == c })
	if inst == nil {
		c.send(errItemNotInInventory)
		return
	}
	price := h.world.Config.Economy.SellPrice(inst.Def)
	if price <= 0 {
		c.send(errItemNotSellable)
		return
	}
	delete(h.items, inst.ID)
	c.Gold += price
	h.log.Info("item_sold", logging.Fields{"player": c.Name, "merchant": n.ID, "item": inst.Def.ID, "price": price, "gold": c.Gold})
	c.send(fmt.Sprintf("OK sold=%s gold=%d", inst.Def.ID, c.Gold))
}

package world

type GameConfig struct {
	StartRoom        string          `json:"start_room"`
	RespawnRoom      string          `json:"respawn_room"`
	RespawnHPPercent int             `json:"respawn_hp_percent"`
	DefaultCharacter string          `json:"default_character"`
	MaxPlayers       int             `json:"max_players"`
	Leveling         LevelingConfig  `json:"leveling"`
	Combat           CombatConfig    `json:"combat"`
	Inventory        InventoryConfig `json:"inventory"`
	Economy          EconomyConfig   `json:"economy"`
}

type EconomyConfig struct {
	StartingGold         int     `json:"starting_gold"`
	SellRatio            float64 `json:"sell_ratio"`
	DeathGoldLossPercent int     `json:"death_gold_loss_percent"`
}

func (e EconomyConfig) SellPrice(it *Item) int {
	return int(float64(it.Price) * e.SellRatio)
}

func BuyPrice(entry ShopEntry, it *Item) int {
	if entry.Price > 0 {
		return entry.Price
	}
	return it.Price
}

type LevelingConfig struct {
	MaxLevel   int `json:"max_level"`
	XPBase     int `json:"xp_base"`
	XPPerLevel int `json:"xp_per_level"`
}

func (c LevelingConfig) XPToNext(level int) int {
	return c.XPBase + c.XPPerLevel*(level-1)
}

type CombatConfig struct {
	MaxEquippedMoves      int     `json:"max_equipped_moves"`
	RestManaRegen         int     `json:"rest_mana_regen"`
	STABMultiplier        float64 `json:"stab_multiplier"`
	CritChance            float64 `json:"crit_chance"`
	CritMultiplier        float64 `json:"crit_multiplier"`
	RefillManaAfterCombat bool    `json:"refill_mana_after_combat"`
}

type InventoryConfig struct {
	EquipmentSlots []EquipSlot `json:"equipment_slots"`
	BagCapacity    int         `json:"bag_capacity"`
}

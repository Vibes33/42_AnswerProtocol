package world

// Archetype is a playable class (archer, fighter, mage, healer).
// Stats grow linearly: StatsAt(level) = BaseStats + GrowthPerLevel * (level - 1).
type Archetype struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Description        string    `json:"description"`
	AllowedTypes       []Element `json:"allowed_types"`
	ManaCostMultiplier float64   `json:"mana_cost_multiplier"`
	BaseStats          Stats     `json:"base_stats"`
	GrowthPerLevel     Stats     `json:"growth_per_level"`
}

func (a *Archetype) StatsAt(level int) Stats {
	return a.BaseStats.Add(a.GrowthPerLevel.Mul(level - 1))
}

// Character is a playable hero: one archetype combined with one type.
type Character struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	Description   string      `json:"description"`
	Archetype     string      `json:"archetype"`
	Type          Element     `json:"type"`
	StartingItems []ItemStack `json:"starting_items,omitempty"`
}

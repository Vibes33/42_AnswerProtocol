package world

type TargetKind string

const (
	TargetEnemy      TargetKind = "enemy"
	TargetAllEnemies TargetKind = "all_enemies"
	TargetSelf       TargetKind = "self"
	TargetAlly       TargetKind = "ally"
	TargetAllAllies  TargetKind = "all_allies"
)

type EffectKind string

const (
	EffectStatus      EffectKind = "status"
	EffectStatMod     EffectKind = "stat_mod"
	EffectHeal        EffectKind = "heal"
	EffectHealPercent EffectKind = "heal_percent"
	EffectRestoreMana EffectKind = "restore_mana"
	EffectCure        EffectKind = "cure"
)

// Effect is a side effect shared by moves and usable items.
// Amount depends on Kind: flat HP (heal), percent of max HP (heal_percent),
// flat mana (restore_mana), or a stat multiplier delta (stat_mod: 0.5 = +50%, -0.2 = -20%).
type Effect struct {
	Kind     EffectKind `json:"kind"`
	Chance   float64    `json:"chance"`
	Target   TargetKind `json:"target,omitempty"`
	Status   string     `json:"status,omitempty"`
	Stat     StatName   `json:"stat,omitempty"`
	Amount   float64    `json:"amount,omitempty"`
	Duration int        `json:"duration,omitempty"`
}

// StatusDef describes a lasting condition applied by an EffectStatus.
// Percentages are of the afflicted entity's max HP, applied at the end of each turn.
type StatusDef struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	DamagePercent   float64 `json:"damage_percent,omitempty"`
	HealPercent     float64 `json:"heal_percent,omitempty"`
	SkipsTurn       bool    `json:"skips_turn,omitempty"`
	WakeChance      float64 `json:"wake_chance,omitempty"`
	DefaultDuration int     `json:"default_duration"`
}

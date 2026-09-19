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

type Effect struct {
	Kind     EffectKind `json:"kind"`
	Chance   float64    `json:"chance"`
	Target   TargetKind `json:"target,omitempty"`
	Status   string     `json:"status,omitempty"`
	Stat     StatName   `json:"stat,omitempty"`
	Amount   float64    `json:"amount,omitempty"`
	Duration int        `json:"duration,omitempty"`
}

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

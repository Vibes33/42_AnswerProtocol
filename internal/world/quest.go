package world

type ObjectiveKind string

const (
	ObjectiveFetch   ObjectiveKind = "fetch"
	ObjectiveDefeat  ObjectiveKind = "defeat"
	ObjectiveDeliver ObjectiveKind = "deliver"
)

type Quest struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Giver         string    `json:"giver"`
	LevelRequired int       `json:"level_required,omitempty"`
	Prerequisites []string  `json:"prerequisites,omitempty"`
	Objective     Objective `json:"objective"`
	Reward        Reward    `json:"reward"`
}

// Objective targets an item id (fetch, deliver) or a monster id (defeat).
// TurnIn is the NPC to report to; empty means the quest giver. For deliver it is the recipient.
type Objective struct {
	Kind   ObjectiveKind `json:"kind"`
	Target string        `json:"target"`
	Count  int           `json:"count"`
	TurnIn string        `json:"turn_in,omitempty"`
}

type Reward struct {
	XP    int         `json:"xp"`
	Gold  int         `json:"gold"`
	Items []ItemStack `json:"items,omitempty"`
}

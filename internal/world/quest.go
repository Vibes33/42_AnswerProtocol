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
	// Dialogue is optional: lines the giver says about this quest instead of its usual dialogue.
	Dialogue *QuestDialogue `json:"dialogue,omitempty"`
}

// QuestDialogue: Offer when the quest is available (TALK, and QUEST when it starts),
// Progress while it is under way, Thanks when it is handed in.
type QuestDialogue struct {
	Offer    string `json:"offer,omitempty"`
	Progress string `json:"progress,omitempty"`
	Thanks   string `json:"thanks,omitempty"`
}

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

package world

// Zone groups rooms. RecommendedLevel is informational only: it never blocks entry.
type Zone struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	RecommendedLevel int    `json:"recommended_level"`
}

type Room struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Zone        string            `json:"zone"`
	Safe        bool              `json:"safe,omitempty"`
	Exits       map[string]string `json:"exits"`
	Items       []ItemStack       `json:"items,omitempty"`
	NPCs        []string          `json:"npcs,omitempty"`
	Monsters    []MonsterSpawn    `json:"monsters,omitempty"`
}

type MonsterSpawn struct {
	Monster        string `json:"monster"`
	Count          int    `json:"count"`
	RespawnSeconds int    `json:"respawn_seconds,omitempty"`
}

package world

// Zone groups rooms. RecommendedLevel is informational only: it never blocks entry.
type Zone struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	RecommendedLevel int    `json:"recommended_level"`
}

// Room is a definition. Items, NPCs and Monsters describe the initial population only;
// what is in the room at runtime lives in the game state.
// A Safe room forbids combat and is a valid respawn point.
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

// MonsterSpawn places Count instances of a monster. RespawnSeconds 0 means it never comes back once defeated.
type MonsterSpawn struct {
	Monster        string `json:"monster"`
	Count          int    `json:"count"`
	RespawnSeconds int    `json:"respawn_seconds,omitempty"`
}

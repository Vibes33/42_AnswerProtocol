package world

// Monster is a hostile creature definition. Each spawn creates an independent runtime instance.
// On defeat, every participant earns XP and Gold, and each Drop is rolled.
type Monster struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Boss        bool     `json:"boss,omitempty"`
	Type        Element  `json:"type"`
	Level       int      `json:"level"`
	Stats       Stats    `json:"stats"`
	Moves       []string `json:"moves"`
	XP          int      `json:"xp"`
	Gold        int      `json:"gold"`
	Drops       []Drop   `json:"drops,omitempty"`
	Dialogue    []string `json:"dialogue,omitempty"`
}

// Drop is rolled once per defeat: with probability Chance, between Min and Max copies drop.
type Drop struct {
	Item   string  `json:"item"`
	Chance float64 `json:"chance"`
	Min    int     `json:"min"`
	Max    int     `json:"max"`
}

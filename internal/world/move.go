package world

import "slices"

type MoveCategory string

const (
	MovePhysical MoveCategory = "physical"
	MoveMagical  MoveCategory = "magical"
	MoveStatus   MoveCategory = "status"
)

// Move is an entry of the global attack pool, usable by players and NPCs.
// A nil Learn rule means the move is reserved to NPCs.
type Move struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Type        Element      `json:"type"`
	Category    MoveCategory `json:"category"`
	Target      TargetKind   `json:"target"`
	Power       int          `json:"power"`
	ManaCost    int          `json:"mana_cost"`
	Accuracy    float64      `json:"accuracy"`
	Effects     []Effect     `json:"effects,omitempty"`
	Learn       *LearnRule   `json:"learn,omitempty"`
}

// LearnRule decides which characters may learn a move, and from which level.
// If Characters is set, the move is a signature move restricted to them.
// Otherwise an empty Archetypes or Types list means "any".
type LearnRule struct {
	Level      int       `json:"level"`
	Archetypes []string  `json:"archetypes,omitempty"`
	Types      []Element `json:"types,omitempty"`
	Characters []string  `json:"characters,omitempty"`
}

func (m *Move) LearnableBy(c *Character, level int) bool {
	r := m.Learn
	if r == nil || level < r.Level {
		return false
	}
	if len(r.Characters) > 0 {
		return slices.Contains(r.Characters, c.ID)
	}
	return (len(r.Archetypes) == 0 || slices.Contains(r.Archetypes, c.Archetype)) &&
		(len(r.Types) == 0 || slices.Contains(r.Types, c.Type))
}

package world

import "slices"

type MoveCategory string

const (
	MovePhysical MoveCategory = "physical"
	MoveMagical  MoveCategory = "magical"
	MoveStatus   MoveCategory = "status"
)

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

type LearnRule struct {
	Level      int       `json:"level"`
	Archetypes []string  `json:"archetypes,omitempty"`
	Types      []Element `json:"types,omitempty"`
	Characters []string  `json:"characters,omitempty"`
}

func (m *Move) LearnableBy(c *Character, level int) bool {
	return m.AvailableTo(c) && level >= m.Learn.Level
}

// AvailableTo reports whether the character can ever learn the move, whatever its level.
func (m *Move) AvailableTo(c *Character) bool {
	r := m.Learn
	if r == nil {
		return false
	}
	if len(r.Characters) > 0 {
		return slices.Contains(r.Characters, c.ID)
	}
	return (len(r.Archetypes) == 0 || slices.Contains(r.Archetypes, c.Archetype)) &&
		(len(r.Types) == 0 || slices.Contains(r.Types, c.Type))
}

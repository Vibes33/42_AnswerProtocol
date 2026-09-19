package world

type Element string

type TypeDef struct {
	ID          Element `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
}

type TypeChart struct {
	Types         []TypeDef                       `json:"types"`
	Effectiveness map[Element]map[Element]float64 `json:"effectiveness"`
}

// Multiplier returns the damage factor of an attack type against a defender type.
// Pairs absent from the chart are neutral (1.0).
func (c *TypeChart) Multiplier(attack, defender Element) float64 {
	if m, ok := c.Effectiveness[attack][defender]; ok {
		return m
	}
	return 1
}

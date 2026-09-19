package world

// World holds every static definition loaded from the data directory.
// It is never mutated after loading, so any goroutine may read it without locking.
type World struct {
	Config     GameConfig
	Types      TypeChart
	Statuses   map[string]*StatusDef
	Archetypes map[string]*Archetype
	Characters map[string]*Character
	Moves      map[string]*Move
	Items      map[string]*Item
	NPCs       map[string]*NPC
	Monsters   map[string]*Monster
	Zones      map[string]*Zone
	Rooms      map[string]*Room
	Quests     map[string]*Quest
}

package world

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

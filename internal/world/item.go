package world

type ItemCategory string

const (
	ItemConsumable ItemCategory = "consumable"
	ItemEquipment  ItemCategory = "equipment"
	ItemMaterial   ItemCategory = "material"
	ItemQuest      ItemCategory = "quest"
	ItemKey        ItemCategory = "key"
	ItemFixture    ItemCategory = "fixture"
)

type Rarity string

const (
	RarityCommon    Rarity = "common"
	RarityUncommon  Rarity = "uncommon"
	RarityRare      Rarity = "rare"
	RarityEpic      Rarity = "epic"
	RarityLegendary Rarity = "legendary"
)

type EquipSlot string

const (
	SlotWeapon    EquipSlot = "weapon"
	SlotArmor     EquipSlot = "armor"
	SlotAccessory EquipSlot = "accessory"
)

type Item struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Description    string       `json:"description"`
	Category       ItemCategory `json:"category"`
	Rarity         Rarity       `json:"rarity"`
	Obtainable     bool         `json:"obtainable"`
	Price          int          `json:"price"`
	LevelRequired  int          `json:"level_required,omitempty"`
	UsableInCombat bool         `json:"usable_in_combat,omitempty"`
	Effects        []Effect     `json:"effects,omitempty"`
	Equipment      *Equipment   `json:"equipment,omitempty"`
}

type Equipment struct {
	Slot       EquipSlot `json:"slot"`
	Bonus      Stats     `json:"bonus"`
	Archetypes []string  `json:"archetypes,omitempty"`
}

type ItemStack struct {
	Item  string `json:"item"`
	Count int    `json:"count"`
}

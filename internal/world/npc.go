package world

import "slices"

type NPCRole string

const (
	RoleDialogue   NPCRole = "dialogue"
	RoleQuestGiver NPCRole = "quest_giver"
	RoleMerchant   NPCRole = "merchant"
	RoleService    NPCRole = "service"
)

type NPC struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Roles       []NPCRole   `json:"roles"`
	Dialogue    []string    `json:"dialogue,omitempty"`
	Quests      []string    `json:"quests,omitempty"`
	Shop        []ShopEntry `json:"shop,omitempty"`
	Service     *Service    `json:"service,omitempty"`
}

func (n *NPC) HasRole(r NPCRole) bool {
	return slices.Contains(n.Roles, r)
}

type ShopEntry struct {
	Item  string `json:"item"`
	Price int    `json:"price,omitempty"`
}

type ServiceKind string

const (
	ServiceRestore     ServiceKind = "restore"
	ServiceClassSelect ServiceKind = "class_select"
)

type Service struct {
	Kind  ServiceKind `json:"kind"`
	Price int         `json:"price"`
}

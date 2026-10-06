package game

import (
	"fmt"
	"strings"
	"unicode"

	"tap/internal/logging"
)

const (
	errNotInGroup     = "ERR 401 NOT_IN_GROUP"
	errAlreadyInGroup = "ERR 402 ALREADY_IN_GROUP"
	errNotInvited     = "ERR 403 NOT_INVITED"
	errPlayerNotFound = "ERR 404 PLAYER_NOT_FOUND"
	errGroupNotFound  = "ERR 404 GROUP_NOT_FOUND"
	errUnknownScope   = "ERR 400 UNKNOWN_SCOPE"
	errInvalidMessage = "ERR 400 INVALID_MESSAGE"
)

type Group struct {
	ID      string
	Leader  *Client
	Members []*Client
	Invited map[string]bool // lowercased player names with a pending invitation
}

func (h *Hub) chat(c *Client, args string) {
	scope, text, _ := strings.Cut(args, " ")
	scope = strings.ToUpper(strings.TrimSpace(scope))
	text = strings.TrimSpace(text)
	if scope == "" || text == "" {
		c.send(errMissingArgument)
		return
	}
	if strings.ContainsFunc(text, unicode.IsControl) {
		c.send(errInvalidMessage)
		return
	}

	event := fmt.Sprintf("EVT %s CHAT %s %s", scope, c.Name, text)
	switch scope {
	case "GLOBAL":
		c.send("OK")
		h.broadcastAll(event)
	case "ROOM":
		c.send("OK")
		h.broadcastRoom(c.Room, event, nil)
	case "GROUP":
		if c.Group == nil {
			c.send(errNotInGroup)
			return
		}
		c.send("OK")
		h.broadcastGroup(c.Group, event)
	default:
		c.send(errUnknownScope)
	}
}

func (h *Hub) group(c *Client, args string) {
	sub, rest, _ := strings.Cut(args, " ")
	switch strings.ToUpper(strings.TrimSpace(sub)) {
	case "":
		c.send(errMissingArgument)
	case "CREATE":
		h.groupCreate(c)
	case "INVITE":
		h.groupInvite(c, rest)
	case "JOIN":
		h.groupJoin(c, rest)
	case "LEAVE":
		h.leaveGroup(c, true)
	case "INFO":
		h.groupInfo(c)
	default:
		c.send(errUnknownCommand)
	}
}

func (h *Hub) groupCreate(c *Client) {
	if c.Group != nil {
		c.send(errAlreadyInGroup)
		return
	}
	h.groupSeq++
	g := &Group{
		ID:      fmt.Sprintf("group.%d", h.groupSeq),
		Leader:  c,
		Members: []*Client{c},
		Invited: map[string]bool{},
	}
	h.groups[g.ID] = g
	c.Group = g
	h.log.Info("group_created", logging.Fields{"player": c.Name, "group": g.ID})
	c.send("OK group=" + g.ID)
}

func (h *Hub) groupInvite(c *Client, name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		c.send(errMissingArgument)
		return
	}
	if c.Group == nil {
		c.send(errNotInGroup)
		return
	}
	target := h.byName[strings.ToLower(name)]
	if target == nil {
		c.send(errPlayerNotFound)
		return
	}
	if target.Group != nil {
		c.send(errAlreadyInGroup)
		return
	}

	c.Group.Invited[strings.ToLower(target.Name)] = true
	c.send("OK")
	target.send("EVT GROUP INVITE " + c.Group.Leader.Name)
}

func (h *Hub) groupJoin(c *Client, leaderName string) {
	leaderName = strings.TrimSpace(leaderName)
	if leaderName == "" {
		c.send(errMissingArgument)
		return
	}
	if c.Group != nil {
		c.send(errAlreadyInGroup)
		return
	}
	leader := h.byName[strings.ToLower(leaderName)]
	if leader == nil {
		c.send(errPlayerNotFound)
		return
	}
	g := leader.Group
	if g == nil || g.Leader != leader {
		c.send(errGroupNotFound)
		return
	}
	if !g.Invited[strings.ToLower(c.Name)] {
		c.send(errNotInvited)
		return
	}

	delete(g.Invited, strings.ToLower(c.Name))
	g.Members = append(g.Members, c)
	c.Group = g
	h.log.Info("group_joined", logging.Fields{"player": c.Name, "group": g.ID})

	c.send("OK group=" + g.ID)
	h.broadcastGroup(g, "EVT GROUP JOIN "+c.Name)
}

func (h *Hub) leaveGroup(c *Client, announce bool) {
	g := c.Group
	if g == nil {
		if announce {
			c.send(errNotInGroup)
		}
		return
	}

	c.Group = nil
	delete(g.Invited, strings.ToLower(c.Name))
	for i, m := range g.Members {
		if m == c {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	if announce {
		c.send("OK")
	}
	h.broadcastGroup(g, "EVT GROUP LEAVE "+c.Name)

	if len(g.Members) == 0 {
		delete(h.groups, g.ID)
		h.log.Info("group_disbanded", logging.Fields{"group": g.ID})
		return
	}
	if g.Leader == c {
		g.Leader = g.Members[0]
		h.log.Info("group_leader_changed", logging.Fields{"group": g.ID, "leader": g.Leader.Name})
		h.broadcastGroup(g, "EVT GROUP LEADER "+g.Leader.Name)
	}
}

func (h *Hub) broadcastGroup(g *Group, msg string) {
	h.broadcast(msg, func(c *Client) bool { return c.Group == g })
}

// groupMember describes an ally for GROUP INFO (extension): where they are and in what state.
type groupMember struct {
	Name      string `json:"name"`
	Character string `json:"character"`
	Room      string `json:"room"`
	RoomName  string `json:"room_name"`
	HP        int    `json:"hp"`
	MaxHP     int    `json:"max_hp"`
	Level     int    `json:"level"`
	Leader    bool   `json:"leader,omitempty"`
}

type groupInfoReply struct {
	ID      string        `json:"id"`
	Leader  string        `json:"leader"`
	Members []groupMember `json:"members"`
}

// groupInfo answers GROUP INFO: the group members, their room and their HP.
func (h *Hub) groupInfo(c *Client) {
	g := c.Group
	if g == nil {
		c.send(errNotInGroup)
		return
	}
	reply := groupInfoReply{ID: g.ID, Leader: g.Leader.Name, Members: []groupMember{}}
	for _, m := range g.Members {
		reply.Members = append(reply.Members, groupMember{
			Name: m.Name, Character: m.Char.Name, Room: m.Room, RoomName: h.world.Rooms[m.Room].Name,
			HP: max(0, m.HP), MaxHP: h.statsOf(m).HP, Level: m.Level, Leader: m == g.Leader,
		})
	}
	h.sendJSON(c, reply)
}

// notifyGroupMove tells the allies that a member changed room (EVT GROUP MOVE, extension), so
// their group panel shows where they are.
func (h *Hub) notifyGroupMove(c *Client) {
	if c.Group == nil {
		return
	}
	g := c.Group
	h.broadcast(fmt.Sprintf("EVT GROUP MOVE %s %s", c.Name, c.Room), func(o *Client) bool { return o.Group == g && o != c })
}

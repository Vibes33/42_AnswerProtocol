package game

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"unicode"
	"unicode/utf8"

	"tap/internal/world"
)

const (
	outBuffer  = 64
	maxNameLen = 32
)

// Error codes 201 and 900 come from RFC 42TAP. The others are extensions:
// the RFC defines no code for these cases (see README, Protocol Implementation).
const (
	errNameInUse        = "ERR 201 NAME_IN_USE"
	errNoExit           = "ERR 301 NO_EXIT"
	errInternal         = "ERR 901 SEND_FAILED"
	errNotConnected     = "ERR 202 NOT_CONNECTED"
	errAlreadyConnected = "ERR 203 ALREADY_CONNECTED"
	errInvalidName      = "ERR 204 INVALID_NAME"
	errUnknownCommand   = "ERR 400 UNKNOWN_COMMAND"
	errMissingArgument  = "ERR 400 MISSING_ARGUMENT"

	errItemNotFound       = "ERR 404 ITEM_NOT_FOUND"
	errItemNotInInventory = "ERR 404 ITEM_NOT_IN_INVENTORY"
	errItemNotTakeable    = "ERR 404 ITEM_NOT_TAKEABLE"
	errInventoryFull      = "ERR 400 INVENTORY_FULL"
)

type Client struct {
	Name string
	Room string
	Addr string
	out  chan string
}

func (c *Client) authenticated() bool { return c.Name != "" }

func (c *Client) send(msg string) bool {
	select {
	case c.out <- msg:
		return true
	default:
		return false
	}
}

type msgKind int

const (
	msgJoin msgKind = iota
	msgCommand
	msgLeave
)

type message struct {
	client *Client
	kind   msgKind
	line   string
}

type Hub struct {
	world    *world.World
	clients  map[*Client]bool
	byName   map[string]*Client
	items    map[string]*ItemInstance
	monsters map[string]*MonsterInstance
	inbox    chan message
}

func NewHub(w *world.World) *Hub {
	h := &Hub{
		world:    w,
		clients:  make(map[*Client]bool),
		byName:   make(map[string]*Client),
		items:    make(map[string]*ItemInstance),
		monsters: make(map[string]*MonsterInstance),
		inbox:    make(chan message, 128),
	}
	h.spawn()
	return h
}

// Run est la boucle de jeu : une seule goroutine, propriétaire de tout l'état mutable.
func (h *Hub) Run() {
	for m := range h.inbox {
		switch m.kind {
		case msgJoin:
			h.clients[m.client] = true
			m.client.send("OK hello proto=1")
		case msgCommand:
			h.handle(m.client, m.line)
		case msgLeave:
			h.remove(m.client)
		}
	}
}

// Serve tourne dans la goroutine de la connexion : elle ne fait que lire des lignes
// et les transmettre au hub. Elle ne touche jamais à l'état du jeu.
func (h *Hub) Serve(conn net.Conn) {
	c := &Client{Addr: conn.RemoteAddr().String(), out: make(chan string, outBuffer)}

	writerDone := make(chan struct{})
	go writeLoop(conn, c.out, writerDone)

	h.inbox <- message{client: c, kind: msgJoin}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 1024), 1024) // RFC 9.4 : 1024 octets par ligne
	for scanner.Scan() {
		h.inbox <- message{client: c, kind: msgCommand, line: strings.TrimSpace(scanner.Text())}
	}

	h.inbox <- message{client: c, kind: msgLeave}
	<-writerDone // le hub ferme c.out, ce qui termine writeLoop
	conn.Close()
}

func writeLoop(conn net.Conn, out <-chan string, done chan<- struct{}) {
	defer close(done)
	for msg := range out {
		if _, err := conn.Write([]byte(msg + "\n")); err != nil {
			return
		}
	}
}

func (h *Hub) handle(c *Client, line string) {
	// The client may already be gone (QUIT, or dropped): its output channel is closed.
	if !h.clients[c] || line == "" {
		return
	}
	verb, args, _ := strings.Cut(line, " ")
	verb = strings.ToUpper(verb)
	log.Printf("recv %s (%s): %s", c.Addr, c.Name, line)

	if !c.authenticated() && verb != "CONNECT" {
		c.send(errNotConnected)
		return
	}

	switch verb {
	case "CONNECT":
		h.connect(c, args)
	case "LOOK":
		h.look(c)
	case "MOVE":
		h.move(c, args)
	case "TAKE":
		h.take(c, args)
	case "DROP":
		h.drop(c, args)
	case "INVENTORY":
		h.inventory(c)
	case "WHO":
		c.send(fmt.Sprintf("OK players=%d", len(h.byName)))
	case "QUIT":
		c.send("OK bye")
		h.remove(c)
	default:
		c.send(errUnknownCommand)
	}
}

func (h *Hub) connect(c *Client, name string) {
	if c.authenticated() {
		c.send(errAlreadyConnected)
		return
	}
	name = strings.TrimSpace(name)
	if !validName(name) {
		c.send(errInvalidName)
		return
	}
	// Names are unique regardless of case, so "alice" and "Alice" cannot coexist.
	key := strings.ToLower(name)
	if _, taken := h.byName[key]; taken {
		c.send(errNameInUse)
		return
	}

	c.Name = name
	c.Room = h.world.Config.StartRoom
	h.byName[key] = c
	log.Printf("connected: %s as %s in %s", c.Addr, c.Name, c.Room)

	c.send("OK connected")
	h.broadcastRoom(c.Room, "EVT ROOM PRESENCE ENTER "+c.Name, c)
	h.broadcastAll(fmt.Sprintf("EVT STATS players=%d", len(h.byName)))
}

// lookReply is the exact JSON shape required by RFC 42TAP 5.1.2.
type lookReply struct {
	Room struct {
		ID          string            `json:"id"`
		Name        string            `json:"name"`
		Description string            `json:"description"`
		Exits       map[string]string `json:"exits"`
	} `json:"room"`
	Players []string `json:"players"`
	Items   []string `json:"items"`
	NPCs    []string `json:"npcs"`
}

func (h *Hub) look(c *Client) {
	room := h.world.Rooms[c.Room]
	var reply lookReply
	reply.Room.ID = room.ID
	reply.Room.Name = room.Name
	reply.Room.Description = room.Description
	reply.Room.Exits = room.Exits
	reply.Players = h.playersIn(c.Room)
	reply.Items = h.itemsIn(c.Room)
	reply.NPCs = h.npcsIn(c.Room)

	payload, err := json.Marshal(reply)
	if err != nil {
		log.Printf("look %s: %v", c.Room, err)
		c.send(errInternal)
		return
	}
	c.send("OK " + string(payload))
}

func (h *Hub) move(c *Client, direction string) {
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction == "" {
		c.send(errMissingArgument)
		return
	}
	dest, ok := h.world.Rooms[c.Room].Exits[direction]
	if !ok {
		c.send(errNoExit)
		return
	}

	from := c.Room
	c.Room = dest
	c.send("OK room=" + dest)
	h.broadcastRoom(from, "EVT ROOM PRESENCE LEAVE "+c.Name, c)
	h.broadcastRoom(dest, "EVT ROOM PRESENCE ENTER "+c.Name, c)
}

func (h *Hub) take(c *Client, query string) {
	if strings.TrimSpace(query) == "" {
		c.send(errMissingArgument)
		return
	}
	inst := h.findItem(query, func(i *ItemInstance) bool { return i.Room == c.Room })
	if inst == nil {
		c.send(errItemNotFound)
		return
	}
	if !inst.Def.Obtainable {
		c.send(errItemNotTakeable)
		return
	}
	if len(h.inventoryOf(c)) >= h.world.Config.Inventory.BagCapacity {
		c.send(errInventoryFull)
		return
	}

	inst.Room = ""
	inst.Holder = c
	log.Printf("%s took %s in %s", c.Name, inst.ID, c.Room)

	c.send("OK taken=" + inst.Def.ID)
	h.broadcastRoom(c.Room, fmt.Sprintf("EVT ROOM ITEM TAKEN %s %s", c.Name, inst.Def.ID), c)
}

func (h *Hub) drop(c *Client, query string) {
	if strings.TrimSpace(query) == "" {
		c.send(errMissingArgument)
		return
	}
	inst := h.findItem(query, func(i *ItemInstance) bool { return i.Holder == c })
	if inst == nil {
		c.send(errItemNotInInventory)
		return
	}

	inst.Holder = nil
	inst.Room = c.Room
	log.Printf("%s dropped %s in %s", c.Name, inst.ID, c.Room)

	c.send("OK dropped=" + inst.Def.ID)
	h.broadcastRoom(c.Room, fmt.Sprintf("EVT ROOM ITEM DROPPED %s %s", c.Name, inst.Def.ID), c)
}

func (h *Hub) inventory(c *Client) {
	ids := []string{}
	for _, inst := range h.inventoryOf(c) {
		ids = append(ids, inst.Def.ID)
	}
	payload, err := json.Marshal(ids)
	if err != nil {
		log.Printf("inventory %s: %v", c.Name, err)
		c.send(errInternal)
		return
	}
	c.send("OK " + string(payload))
}

func validName(name string) bool {
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return false
	}
	for _, r := range name {
		if unicode.IsSpace(r) || !unicode.IsGraphic(r) {
			return false
		}
	}
	return true
}

// remove drops every trace of a client before announcing its departure, as the subject requires.
// It is safe to call twice: QUIT removes the client, then the closed connection asks again.
func (h *Hub) remove(c *Client) {
	if !h.clients[c] {
		return
	}
	delete(h.clients, c)
	name, room := c.Name, c.Room
	if name != "" {
		delete(h.byName, strings.ToLower(name))
		h.dropAll(c, room)
	}
	close(c.out) // ends writeLoop, which unblocks Serve, which closes the socket

	if name == "" {
		log.Printf("disconnected: %s", c.Addr)
		return
	}
	log.Printf("disconnected: %s (%s)", c.Addr, name)
	h.broadcastRoom(room, "EVT ROOM PRESENCE LEAVE "+name, nil)
	h.broadcastAll(fmt.Sprintf("EVT STATS players=%d", len(h.byName)))
}

func (h *Hub) broadcastAll(msg string) {
	h.broadcast(msg, func(*Client) bool { return true })
}

func (h *Hub) broadcastRoom(room, msg string, except *Client) {
	h.broadcast(msg, func(c *Client) bool { return c != except && c.Room == room })
}

// broadcast never blocks: a client whose buffer is full is dropped after the loop,
// so one dead client cannot interrupt delivery to the others.
func (h *Hub) broadcast(msg string, want func(*Client) bool) {
	var stuck []*Client
	for c := range h.clients {
		if !c.authenticated() || !want(c) {
			continue
		}
		if !c.send(msg) {
			stuck = append(stuck, c)
		}
	}
	for _, c := range stuck {
		log.Printf("dropping %s (%s): output buffer full", c.Addr, c.Name)
		h.remove(c)
	}
}

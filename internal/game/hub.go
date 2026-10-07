package game

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"tap/internal/logging"
	"tap/internal/world"
)

const (
	outBuffer  = 64
	maxNameLen = 32

	// Abuse detection thresholds (subject: monitor command flooding and rapid connections).
	floodWindow = time.Second
	floodLimit  = 20
	connWindow  = 10 * time.Second
	connLimit   = 5
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
	errUnknownCharacter = "ERR 205 UNKNOWN_CHARACTER"
	errUnknownCommand   = "ERR 400 UNKNOWN_COMMAND"
	errMissingArgument  = "ERR 400 MISSING_ARGUMENT"

	errItemNotFound       = "ERR 404 ITEM_NOT_FOUND"
	errItemNotInInventory = "ERR 404 ITEM_NOT_IN_INVENTORY"
	errItemNotTakeable    = "ERR 404 ITEM_NOT_TAKEABLE"
	errInventoryFull      = "ERR 400 INVENTORY_FULL"
)

type Client struct {
	Name  string
	Room  string
	Addr  string
	Group *Group
	out   chan string

	Char     *world.Character
	Level    int
	XP       int
	HP       int
	Mana     int
	Gold     int
	Statuses map[string]int             // status id -> remaining turns
	Equipped map[world.EquipSlot]string // slot -> worn item instance id
	Buffs    map[world.StatName]*buff   // bonus temporaires de combat (Peau de pierre, Cri de guerre…)
	Quests   map[string]*QuestProgress
	Moves    []string        // equipped attacks, at most combat.max_equipped_moves
	Offers   []string        // unlocked attacks waiting for LEARN (replace one, or skip)
	seen     map[string]bool // attacks already unlocked: equipped, offered, skipped or forgotten

	talkIndex map[string]int // npc id -> next dialogue line

	log         *logging.Logger
	windowStart time.Time // start of the current flood-detection window
	windowCount int
	warned      bool
}

func (c *Client) authenticated() bool { return c.Name != "" }

func (c *Client) send(msg string) bool {
	select {
	case c.out <- msg:
		c.logSent(msg)
		return true
	default:
		return false
	}
}

// logSent records everything leaving the server: replies, error codes and events.
func (c *Client) logSent(msg string) {
	f := logging.Fields{"player": c.Name, "addr": c.Addr}
	switch {
	case strings.HasPrefix(msg, "EVT "):
		f["message"] = msg
		c.log.Info("event", f)
	case strings.HasPrefix(msg, "ERR "):
		parts := strings.SplitN(msg, " ", 3)
		f["code"] = parts[1]
		if len(parts) > 2 {
			f["message"] = parts[2]
		}
		c.log.Warn("error_reply", f)
	default:
		f["reply"] = msg
		c.log.Info("response", f)
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
	groups   map[string]*Group
	groupSeq int
	seq      int // instance id counter, shared by items and monsters
	inbox    chan message
	log      *logging.Logger
	recent   map[string][]time.Time // client host -> recent connection times
}

func NewHub(w *world.World, lg *logging.Logger) *Hub {
	h := &Hub{
		world:    w,
		clients:  make(map[*Client]bool),
		byName:   make(map[string]*Client),
		items:    make(map[string]*ItemInstance),
		monsters: make(map[string]*MonsterInstance),
		groups:   make(map[string]*Group),
		inbox:    make(chan message, 128),
		log:      lg,
		recent:   make(map[string][]time.Time),
	}
	h.spawn()
	return h
}

// noteConnection flags hosts that reconnect abnormally often.
func (h *Hub) noteConnection(c *Client) {
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		host = c.Addr
	}
	now := time.Now()
	kept := h.recent[host][:0]
	for _, t := range h.recent[host] {
		if now.Sub(t) < connWindow {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	h.recent[host] = kept

	h.log.Info("connection", logging.Fields{"addr": c.Addr})
	if len(kept) > connLimit {
		h.log.Warn("rapid_connections", logging.Fields{
			"host": host, "count": len(kept), "window_seconds": connWindow.Seconds(),
		})
	}
}

// noteCommand counts commands per second and warns once per window when a client floods.
func (h *Hub) noteCommand(c *Client, verb, args string) {
	now := time.Now()
	if now.Sub(c.windowStart) > floodWindow {
		c.windowStart, c.windowCount, c.warned = now, 0, false
	}
	c.windowCount++

	h.log.Info("command", logging.Fields{
		"addr": c.Addr, "player": c.Name, "verb": verb, "args": args,
	})
	if c.windowCount > floodLimit && !c.warned {
		c.warned = true
		h.log.Warn("command_flood", logging.Fields{
			"addr": c.Addr, "player": c.Name, "commands": c.windowCount, "window_seconds": floodWindow.Seconds(),
		})
	}
}

// Run is the game loop: a single goroutine that owns all the mutable state.
func (h *Hub) Run() {
	for m := range h.inbox {
		switch m.kind {
		case msgJoin:
			h.clients[m.client] = true
			h.noteConnection(m.client)
			m.client.send("OK hello proto=1")
		case msgCommand:
			h.handle(m.client, m.line)
		case msgLeave:
			h.remove(m.client)
		}
	}
}

// Serve runs in the connection's goroutine: it only reads lines and hands them to the hub.
// It never touches the game state.
func (h *Hub) Serve(conn net.Conn) {
	c := &Client{Addr: conn.RemoteAddr().String(), out: make(chan string, outBuffer), log: h.log}

	writerDone := make(chan struct{})
	go writeLoop(conn, c.out, writerDone)

	h.inbox <- message{client: c, kind: msgJoin}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 1024), 1024) // RFC 9.4 : 1024 octets par ligne
	for scanner.Scan() {
		h.inbox <- message{client: c, kind: msgCommand, line: strings.TrimSpace(scanner.Text())}
	}

	h.inbox <- message{client: c, kind: msgLeave}
	<-writerDone // the hub closes c.out, which ends writeLoop
	conn.Close()
}

func writeLoop(conn net.Conn, out <-chan string, done chan<- struct{}) {
	defer close(done)
	// Closing here (after the last message, e.g. "OK bye") unblocks Serve's read: otherwise,
	// after QUIT, the connection would stay open until the client cut it.
	defer conn.Close()
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
	h.noteCommand(c, verb, args)
	h.reviveMonsters()

	// CHARACTERS is allowed before CONNECT: the client shows the character choice on the
	// login screen, next to the name.
	if !c.authenticated() && verb != "CONNECT" && verb != "CHARACTERS" {
		c.send(errNotConnected)
		return
	}

	switch verb {
	case "CONNECT":
		h.connect(c, args)
	case "CHARACTERS":
		h.characters(c)
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
	case "CHAT":
		h.chat(c, args)
	case "GROUP":
		h.group(c, args)
	case "TALK":
		h.talk(c, args)
	case "ATTACK":
		h.attack(c, args)
	case "STATUS":
		h.status(c)
	case "LEARN":
		h.learn(c, args)
	case "QUEST":
		h.quest(c, args)
	case "QUESTS":
		h.quests(c)
	case "USE":
		h.use(c, args)
	case "SHOP":
		h.shop(c)
	case "BUY":
		h.buy(c, args)
	case "SELL":
		h.sell(c, args)
	case "WHO":
		c.send(fmt.Sprintf("OK players=%d", len(h.byName)))
	case "QUIT":
		c.send("OK bye")
		h.remove(c)
	default:
		c.send(errUnknownCommand)
	}
}

// connect accepts "CONNECT <name> [<character>]". The character (id or name) is an extension:
// an RFC client only sends the name and gets the default character. A name never contains a
// space, so the second word cannot be part of it.
func (h *Hub) connect(c *Client, args string) {
	if c.authenticated() {
		c.send(errAlreadyConnected)
		return
	}
	name, choice, _ := strings.Cut(strings.TrimSpace(args), " ")
	if !validName(name) {
		c.send(errInvalidName)
		return
	}
	char := h.world.Characters[h.world.Config.DefaultCharacter]
	if choice = strings.TrimSpace(choice); choice != "" {
		if char = h.findCharacter(choice); char == nil {
			c.send(errUnknownCharacter)
			return
		}
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
	h.initPlayer(c, char)
	h.log.Info("player_connected", logging.Fields{"addr": c.Addr, "player": c.Name, "room": c.Room, "character": char.ID})

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
		h.log.Error("encode_failed", logging.Fields{"command": "LOOK", "room": c.Room, "error": err.Error()})
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
	h.notifyGroupMove(c)
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
	h.log.Info("item_taken", logging.Fields{"player": c.Name, "item": inst.ID, "room": c.Room})

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
	h.log.Info("item_dropped", logging.Fields{"player": c.Name, "item": inst.ID, "room": c.Room})

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
		h.log.Error("encode_failed", logging.Fields{"command": "INVENTORY", "player": c.Name, "error": err.Error()})
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
		h.leaveGroup(c, false)
	}
	close(c.out) // ends writeLoop, which unblocks Serve, which closes the socket

	if name == "" {
		h.log.Info("disconnected", logging.Fields{"addr": c.Addr})
		return
	}
	h.log.Info("player_disconnected", logging.Fields{"addr": c.Addr, "player": name, "room": room})
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
		h.log.Warn("client_dropped", logging.Fields{
			"addr": c.Addr, "player": c.Name, "reason": "output buffer full",
		})
		h.remove(c)
	}
}

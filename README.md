*This project has been created as part of the 42 curriculum by <rydelepi>, <nicolasd>, <guifouqu>.*

# TAP — The Answer Protocol

## Description

TAP is a small multiplayer text adventure (MUD) built in Go. A TCP server hosts a shared world
where players explore rooms, chat, pick up items, fight enemies and complete quests in real time.
Two clients are provided: a command-line client and a graphical client. All components speak the
line-based protocol defined in RFC 42TAP.

_TODO: short overview of the world and the main gameplay features._

## Instructions

Requirements: Go 1.27+ and `make`. The GUI client is a local web interface (standard library only,
no C compiler needed).

```bash
make deps
make build
make run-server
make run-client       # in another terminal
make run-client-gui   # in another terminal (opens http://127.0.0.1:8043)
```

See [Building and Running](#building-and-running) for details.

**Playing over the local network.** `make run-server` also serves the web client to every computer of the
network, on port 8042, and prints the addresses to share, such as `http://192.168.1.20:8042`. The other
players open that address in their browser, with nothing to install. Players who have the project can also
use their own clients: `make run-client ADDR=192.168.1.20:4242` or `make run-client-gui ADDR=192.168.1.20:4242`.
`ARGS="-web :9000"` changes the web port, `ARGS="-web="` turns the web client off.

## Architecture

_TODO: dispatcher/router vs inline handling, package layout, concurrency model
(central hub goroutine + per-player writer goroutine), why the static world is immutable._

## Protocol Implementation

_TODO: list every deviation from or extension to RFC 42TAP and justify it.
Extensions never remove or rename RFC response fields, never require non-RFC commands to play,
and can be disabled with the server's strict mode._

- **Extra error codes.** RFC 42TAP defines no code for these situations, so the following were added,
  keeping the RFC's numbering logic (2xx = connection, 4xx = request):

  | Code | Message | When |
  |---|---|---|
  | 202 | `NOT_CONNECTED` | any command other than `CONNECT` sent before authenticating |
  | 203 | `ALREADY_CONNECTED` | `CONNECT` sent twice on the same connection |
  | 204 | `INVALID_NAME` | empty name, name over 32 characters, or containing spaces or control characters |
  | 205 | `UNKNOWN_CHARACTER` | `CONNECT <name> <character>` names a character that does not exist |
  | 403 | `LEVEL_TOO_LOW` | `USE` on equipment that requires a higher level |
  | 403 | `WRONG_CLASS` | `USE` on equipment the character's class cannot wear |
  | 409 | `ITEM_NOT_USABLE` | `USE` on an item with no effect and no equipment slot (materials, fixtures) |
  | 403 | `MOVE_NOT_LEARNED` | `ATTACK <npc> <move>` with a move that is not in one of the character's attack slots |
  | 404 | `MOVE_NOT_OFFERED` | `LEARN <move>` with a move that is not waiting to be learned |
  | 409 | `MOVE_SLOTS_FULL` | `LEARN <move>` without naming the attack to replace while every slot is taken |
  | 404 | `MOVE_NOT_FOUND` | `ATTACK <npc> <move>` with an unknown move id |
  | 404 | `NO_MERCHANT` | `SHOP`, `BUY` or `SELL` in a room without a merchant |
  | 404 | `ITEM_NOT_SOLD` | `BUY` an item the merchant does not sell |
  | 407 | `NOT_ENOUGH_GOLD` | `BUY` without enough gold |
  | 408 | `NOT_ENOUGH_MANA` | `ATTACK <npc> <move>` without enough mana for the move |
  | 409 | `ITEM_NOT_SELLABLE` | `SELL` an item that has no resale value (quest rewards) |
  | 400 | `UNKNOWN_COMMAND` | unknown verb |
  | 400 | `MISSING_ARGUMENT` | a command was sent without its required argument (`MOVE` with no direction) |
  | 400 | `INVENTORY_FULL` | `TAKE` while carrying `inventory.bag_capacity` items |
  | 404 | `ITEM_NOT_TAKEABLE` | `TAKE` on an item whose `obtainable` is false (scenery such as the notice board) |

- **Two extra events.** `EVT ROOM ITEM TAKEN <player> <item-id>` and
  `EVT ROOM ITEM DROPPED <player> <item-id>` are pushed to the other players in the room, so a GUI
  can refresh its room view without polling `LOOK`. RFC 42TAP defines no item event; clients that
  ignore unknown events are unaffected.

- **Items are never lost.** When a player disconnects, everything they carried drops in the room
  where they stood, so quest items cannot disappear from the world.

- **`LOOK` reports definition ids.** Items and NPCs are listed by their definition id
  (`item.healing_herb`), as in the RFC examples, not by the server's internal instance id. Two copies
  of the same item in a room therefore appear twice in the list. Each copy is still a distinct
  instance server-side, so taking one leaves the other.

- **Choosing a character.** `CHARACTERS` (allowed before `CONNECT`) returns the playable characters as JSON:
  id, name, class, element and base stats. `CONNECT <name> [<character>]` accepts an optional character id
  or name (`CONNECT ada char.pyra`, `CONNECT ada Pyra`). A name never contains a space, so the second word is
  unambiguous; a plain RFC `CONNECT <name>` gets the default character (`default_character` in
  `data/game.json`). The web client only adds the character when the server answered `CHARACTERS`, so it still
  sends a plain `CONNECT` to other groups' servers.

- **Choosing an attack.** `ATTACK <npc>` keeps the RFC behaviour (basic attack). `ATTACK <npc> <move-id>`
  (`ATTACK mob.green_blob move.rock_smash`) uses one of the character's equipped moves and costs its mana.
  No NPC name starts with `move.`, so the last word is never ambiguous. The reply adds `move` and `mana`.

- **Three attack slots.** A character carries at most `combat.max_equipped_moves` attacks (`data/game.json`,
  default 3). The attacks unlocked at level 1 fill the slots; an attack unlocked later takes a free slot, or,
  when every slot is taken, waits as an offer (`move_offers` in `STATUS`, and a line in the `ATTACK`/`QUEST`
  log). `LEARN <move-id> <replaced-move-id>` learns it in place of an equipped attack, `LEARN <move-id> skip`
  turns it down, and `LEARN <move-id>` alone takes a free slot. A replaced or skipped attack is never offered
  again. Replies: `OK learned=<id> forgot=<id>`, `OK skipped=<id>`, `OK learned=<id>`.

- **Using items.** `USE <item>` applies a consumable's effects (heal, mana, cure…) and uses it up, or
  equips a piece of equipment, whose bonuses then add to the player's stats; using worn equipment again takes
  it off. The reply is JSON: `item`, `action` (`used`, `equipped` or `unequipped`), the new `hp`/`max_hp` and
  `mana`/`max_mana`, and a `log`. `STATUS` adds `equipment` (slot → item id) and `bag` (each kind of item with
  its name, description, category, count, and whether it is usable or equipped).

- **Merchants.** `SHOP` lists what the merchant of the room sells and what it buys back from the player;
  `BUY <item>` answers `OK bought=<item-id> gold=<gold>` and `SELL <item>` answers `OK sold=<item-id> gold=<gold>`.

- **Group play.** `GROUP INFO` returns the members of the player's group with their room and health.
  `EVT GROUP MOVE <player> <room-id>` tells the group when a member changes room, and
  `EVT GROUP XP <killer> <monster-id> <xp>` tells a member it received a share of experience: the XP of a
  defeated monster is split between the group members present in the room (+10 % per ally).

- **`STATUS` describes the character.** Besides the RFC fields, the reply carries `character`, `class`,
  `element` and `moves` (every attack the character can learn, with its power, mana cost and the level that
  unlocks it, plus `equipped` and `offered`), `move_slots`, `equipped_moves` (attack ids in slot order) and
  `move_offers`, so clients can show who the player is and which attacks they have.

- **Monsters are listed as NPCs.** RFC 42TAP has a single `npcs` field, so `LOOK` lists peaceful NPCs
  and hostile monsters together. They are separate files in the world data (`npcs.json`,
  `monsters.json`), but the protocol does not distinguish them.

- **Case-insensitive player names.** `alice` and `Alice` are the same name, so the second one gets
  `ERR 201 NAME_IN_USE`. The RFC only requires uniqueness; this avoids two players being told apart
  by capitalisation alone.

- **Connection limit.** The server accepts at most `max_players` simultaneous clients
  (`data/game.json`, default 4; `0` means unlimited). A client connecting while the server is full
  receives `ERR 900 SERVER_FULL` instead of the `OK hello proto=1` greeting, and the connection is
  closed. RFC 42TAP defines no limit and no dedicated code; `900` is reused because it already means
  "connection establishment failed", so any RFC client treats it as a fatal connection error.

## Combat System

_TODO: turn-based mechanics, initiative order, damage formulas, counterattacks, respawn rules,
additional combat commands (DEFEND, FLEE, ...) and the reasoning behind them._

## Quest System

_TODO: quest progression tracking, completion validation, reward distribution._

## World Design

_TODO: map layout (rooms, loops, optional branch), NPC roles, item distribution._

## Server Logging

_TODO: JSON log format, levels, logged event types, output destinations,
how to monitor the server and detect abuse (command flooding, rapid reconnections)._

## Building and Running

| Target | Description |
|---|---|
| `make deps` / `make install` | Download and tidy Go module dependencies |
| `make build` | Build all binaries into `bin/` |
| `make run-client` | Run the CLI client (`ADDR=host:port` to override, default `localhost:4242`) |
| `make run-server` | Run the game server (port 4242) and the web client for the whole network (port 8042) |
| `make run-client-gui` | Run the web GUI client alone, on http://127.0.0.1:8043 (`ADDR=host:port` to play on another server, `ARGS="-no-open"` to skip opening the browser) |
| `make lint` | `go vet` + `gofmt` check |
| `make fmt` | Format all Go sources |
| `make test` | Run tests with the race detector |
| `make clean` | Remove build artifacts and caches |

Extra arguments can be passed with `ARGS`, e.g. `make run-server ARGS="-data path/to/world"`.

The server loads every JSON file of the world directory (`-data`, default `data/`) at startup and
validates all cross-references. If the world is invalid, it prints every problem found and exits
with status 1 instead of starting.

## Testing

_TODO: how to test multiplayer (several clients, `nc`), combat, quests,
TCP fragmentation/coalescing, abrupt disconnects._

## Group Contributions

| Member | Responsibilities |
|---|---|
| `<login1>` | _TODO_ |
| `<login2>` | _TODO_ |
| `<login3>` | _TODO_ |

## Resources

- RFC 42TAP — The Answer Protocol (provided with the subject)
- [Go `net` package](https://pkg.go.dev/net)
- [Effective Go](https://go.dev/doc/effective_go)
- [Go Concurrency Patterns (Go blog)](https://go.dev/blog/pipelines)
- [RFC 2119 — Requirement levels](https://www.rfc-editor.org/rfc/rfc2119)
- [RFC 5234 — ABNF](https://www.rfc-editor.org/rfc/rfc5234)

### AI usage

_TODO: which tasks and which parts of the project AI was used for._

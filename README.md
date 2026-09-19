*This project has been created as part of the 42 curriculum by <login1>, <login2>, <login3>.*

# TAP — The Answer Protocol

## Description

TAP is a small multiplayer text adventure (MUD) built in Go. A TCP server hosts a shared world
where players explore rooms, chat, pick up items, fight enemies and complete quests in real time.
Two clients are provided: a command-line client and a graphical client. All components speak the
line-based protocol defined in RFC 42TAP.

_TODO: short overview of the world and the main gameplay features._

## Instructions

Requirements: Go 1.27+ and `make`. The GUI client additionally needs a C compiler
(`xcode-select --install` on macOS).

```bash
make deps
make build
make run-server
make run-client
```

See [Building and Running](#building-and-running) for details.

## Architecture

_TODO: dispatcher/router vs inline handling, package layout, concurrency model
(central hub goroutine + per-player writer goroutine), why the static world is immutable._

## Protocol Implementation

_TODO: list every deviation from or extension to RFC 42TAP and justify it.
Extensions never remove or rename RFC response fields, never require non-RFC commands to play,
and can be disabled with the server's strict mode._

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
| `make run-server` | Run the server |
| `make run-client` | Run the CLI client (`ADDR=host:port` to override, default `localhost:4242`) |
| `make run-client-gui` | Run the GUI client |
| `make lint` | `go vet` + `gofmt` check |
| `make fmt` | Format all Go sources |
| `make test` | Run tests with the race detector |
| `make clean` | Remove build artifacts and caches |

Extra arguments can be passed with `ARGS`, e.g. `make run-server ARGS="-world data/world.yaml"`.

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

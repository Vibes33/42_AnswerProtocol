package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"

	"tap/internal/game"
	"tap/internal/world"
)

const listenAddr = ":4242"

func main() {
	dataDir := flag.String("data", "data", "directory containing the world JSON files")
	flag.Parse()

	w, err := loadWorld(*dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid world in %s:\n%v\n", *dataDir, err)
		os.Exit(1)
	}
	log.Printf("world loaded from %s: %d rooms, %d npcs, %d monsters, %d items, %d quests",
		*dataDir, len(w.Rooms), len(w.NPCs), len(w.Monsters), len(w.Items), len(w.Quests))

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	hub := game.NewHub(w)
	go hub.Run()

	var slots limiter
	if w.Config.MaxPlayers > 0 {
		slots = make(limiter, w.Config.MaxPlayers)
	}

	log.Printf("TAP server listening on %s (max clients: %s)", listenAddr, slots)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		if !slots.acquire() {
			log.Printf("rejected %s: server full (%d/%d)", conn.RemoteAddr(), len(slots), cap(slots))
			go reject(conn, "ERR 900 SERVER_FULL")
			continue
		}
		go func() {
			defer slots.release()
			hub.Serve(conn)
		}()
	}
}

// limiter caps simultaneous clients: each connection holds one slot of the buffered channel
// until it disconnects. A nil limiter means unlimited.
type limiter chan struct{}

func (l limiter) acquire() bool {
	if l == nil {
		return true
	}
	select {
	case l <- struct{}{}:
		return true
	default:
		return false
	}
}

func (l limiter) release() {
	if l != nil {
		<-l
	}
}

func (l limiter) String() string {
	if l == nil {
		return "unlimited"
	}
	return fmt.Sprint(cap(l))
}

func reject(conn net.Conn, msg string) {
	defer conn.Close()
	if _, err := conn.Write([]byte(msg + "\n")); err != nil {
		log.Printf("reject %s: %v", conn.RemoteAddr(), err)
	}
}

func loadWorld(dir string) (*world.World, error) {
	w, err := world.Load(dir)
	if err != nil {
		return nil, err
	}
	if err := w.Validate(); err != nil {
		return nil, err
	}
	return w, nil
}

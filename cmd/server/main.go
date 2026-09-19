package main

import (
	"bufio"
	"log"
	"net"
	"strings"
)

const listenAddr = ":4242"

func main() {
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	log.Printf("TAP server listening on %s", listenAddr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept: %v", err)
			continue
		}
		go handleConn(conn)
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()

	addr := conn.RemoteAddr().String()
	log.Printf("connected: %s", addr)
	defer log.Printf("disconnected: %s", addr)

	send(conn, "OK hello proto=1")

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		log.Printf("recv %s: %s", addr, line)

		verb, args, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "CONNECT":
			if args == "" {
				send(conn, "ERR 400 missing player name")
				continue
			}
			send(conn, "OK connected")
		case "QUIT":
			send(conn, "OK bye")
			return
		default:
			send(conn, "ERR 404 unknown command")
		}
	}
}

func send(conn net.Conn, msg string) {
	if _, err := conn.Write([]byte(msg + "\n")); err != nil {
		log.Printf("send: %v", err)
	}
}

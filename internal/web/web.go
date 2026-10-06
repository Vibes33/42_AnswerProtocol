// Package web is TAP's graphical client: it serves the web interface (ui/) and relays its
// commands to the game server over TCP.
//
//	browser ──POST /send──▶ relay ──TCP──▶ game server
//	browser ◀──SSE /events── relay ◀──TCP── game server
//
// The relay does not understand the protocol: it carries raw RFC 42TAP lines both ways. All
// the logic (reply queue, JSON parsing, events) lives in ui/app.js, which keeps the client
// interchangeable with the other groups' servers.
//
// Each tab has its own session: its TCP connection to the game server, identified by a random
// token. Several players can therefore share one relay, including from other computers of the
// local network. The relay runs inside tap-server (the server serves its own client) or on its
// own, with cmd/gui, to play on another group's server.
package web

import (
	"bufio"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed ui
var uiFiles embed.FS

// maxLine mirrors the server limit (RFC 9.4: 1024 bytes per line).
const maxLine = 1024

// relay keeps one session per open tab.
type relay struct {
	addr     string
	mu       sync.Mutex // guards sessions
	sessions map[string]*session
}

// session is a tab's TCP connection to the game server.
type session struct {
	mu   sync.Mutex // serialises writes to keep the order of commands
	conn net.Conn
}

// Handler serves the web interface and relays each tab to the game server gameAddr.
func Handler(gameAddr string) http.Handler {
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		panic(err) // ui/ is embedded in the binary: cannot happen at run time
	}
	r := &relay{addr: gameAddr, sessions: map[string]*session{}}

	mux := http.NewServeMux()
	mux.Handle("GET /", http.FileServerFS(ui))
	mux.HandleFunc("GET /config", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"addr": shownAddr(gameAddr, req)})
	})
	mux.HandleFunc("GET /events", r.events)
	mux.HandleFunc("POST /send", r.send)
	return mux
}

// shownAddr is the game server address shown to the player. A relay that reaches the game
// locally ("localhost:4242") shows the address the browser came through instead: a player on
// another computer sees "10.18.188.175:4242", not "localhost".
func shownAddr(gameAddr string, req *http.Request) string {
	host, port, err := net.SplitHostPort(gameAddr)
	if err != nil || (host != "" && host != "localhost" && !net.ParseIP(host).IsLoopback()) {
		return gameAddr
	}
	if h, _, err := net.SplitHostPort(req.Host); err == nil {
		return net.JoinHostPort(h, port)
	}
	return gameAddr
}

// events opens a new session: the connection to the game server, whose every line is pushed
// to the browser as an SSE event. The first event gives the browser its session token. The
// stream ends when the server or the tab closes.
func (r *relay) events(w http.ResponseWriter, req *http.Request) {
	if !sameOrigin(req) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")

	conn, err := net.DialTimeout("tcp", r.addr, 5*time.Second)
	if err != nil {
		fmt.Fprintf(w, "event: fail\ndata: %s\n\n", oneLine(err.Error()))
		flusher.Flush()
		return
	}

	id := newToken()
	r.mu.Lock()
	r.sessions[id] = &session{conn: conn}
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.sessions, id)
		r.mu.Unlock()
		conn.Close()
	}()

	// Tab closed: cut the TCP connection; the server removes the player and announces the departure.
	go func() {
		<-req.Context().Done()
		conn.Close()
	}()

	fmt.Fprintf(w, "event: session\ndata: %s\n\n", id)
	flusher.Flush()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		fmt.Fprintf(w, "data: %s\n\n", strings.TrimRight(scanner.Text(), "\r"))
		flusher.Flush()
	}
	fmt.Fprint(w, "event: closed\ndata: eof\n\n")
	flusher.Flush()
}

// send writes a command (one line) on the TCP connection of the tab's session.
func (r *relay) send(w http.ResponseWriter, req *http.Request) {
	if !sameOrigin(req) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	r.mu.Lock()
	s := r.sessions[req.Header.Get("X-TAP-Session")]
	r.mu.Unlock()
	if s == nil {
		http.Error(w, "not connected", http.StatusServiceUnavailable)
		return
	}

	body, err := io.ReadAll(io.LimitReader(req.Body, maxLine))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	line := oneLine(strings.TrimSpace(string(body)))

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.conn.Write([]byte(line + "\n")); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// newToken draws an unpredictable token: on the local network, a player must not be able to
// send commands into someone else's session.
func newToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// LANIPs lists the machine's IPv4 addresses on the local network, to share with the other players.
func LANIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, a := range addrs {
		if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
			ips = append(ips, n.IP.String())
		}
	}
	return ips
}

// oneLine makes sure a message fits on a single line (protocol framing).
func oneLine(s string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(s)
}

// sameOrigin rejects requests sent by another site open in the browser (CSRF).
func sameOrigin(req *http.Request) bool {
	origin := req.Header.Get("Origin")
	return origin == "" || origin == "http://"+req.Host
}

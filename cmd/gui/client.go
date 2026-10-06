package main

import (
	"bufio"
	"net"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

// client est la connexion TCP au serveur.
// Le protocole ne numérote pas les réponses : le serveur répond aux commandes dans l'ordre
// où il les reçoit. On garde donc la file des commandes envoyées, et chaque ligne OK/ERR
// reçue correspond à la plus ancienne commande en attente. Les lignes EVT n'y entrent pas.
type client struct {
	conn    net.Conn
	pending []string // commandes envoyées sans réponse, dans l'ordre d'envoi
	greeted bool     // la première ligne du serveur est le greeting, pas une réponse
}

func dial(addr string) (*client, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &client{conn: conn}, nil
}

// send n'est appelée que depuis la goroutine de l'interface, comme popCommand :
// la file pending n'a donc pas besoin de mutex.
func (c *client) send(line string) error {
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		return err
	}
	c.pending = append(c.pending, line)
	return nil
}

// popCommand renvoie la commande à laquelle la réponse reçue correspond.
func (c *client) popCommand() string {
	if len(c.pending) == 0 {
		return ""
	}
	cmd := c.pending[0]
	c.pending = c.pending[1:]
	return cmd
}

// readLoop tourne dans sa propre goroutine pour que l'interface ne bloque jamais en lecture.
// Fyne interdit de toucher aux widgets hors de sa goroutine : chaque ligne est donc
// transmise via fyne.Do, qui exécute la fonction dans la goroutine de l'interface.
func (c *client) readLoop(onLine func(string), onClose func()) {
	scanner := bufio.NewScanner(c.conn)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		fyne.Do(func() { onLine(line) })
	}
	fyne.Do(onClose)
}

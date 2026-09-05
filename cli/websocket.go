package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

// reconnectPongWait bounds how long the reader waits for any frame before
// treating the connection as dead. The server pings every 54s, so healthy
// connections always see traffic well inside this window.
const reconnectPongWait = 90 * time.Second

const (
	handshakeTimeout = 10 * time.Second

	maxReconnectBackoff = 30 * time.Second
)

type Connection struct {
	conn     *websocket.Conn
	base     string        // server URL without the /ws suffix
	Send     chan Message  // buffered channel for writes
	firstMsg *Message      // buffered first message (used after password check)
	done     chan struct{} // signal to stop writePump

	password string // effective room password, reused on reconnect
}

// connErrMsg reports a dead read side; the TUI trades it for a reconnect.
type connErrMsg struct{ err error }

// reconnectedMsg carries the replacement connection after a successful
// rejoin; the server history replay resyncs the room.
type reconnectedMsg struct{ conn *Connection }

// connFatalMsg ends the session: the server rejected the rejoin.
type connFatalMsg struct{ err error }

func connectWebSocket(server string) (*Connection, error) {
	dialer := &websocket.Dialer{
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: handshakeTimeout,
	}

	conn, _, err := dialer.Dial(server, nil)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", server, err)
	}

	armReadDeadline(conn)
	conn.SetPingHandler(func(appData string) error {
		armReadDeadline(conn)
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(time.Second))
	})

	return &Connection{
		conn: conn,
		base: strings.TrimSuffix(server, "/ws"),
		Send: make(chan Message, 32),
		done: make(chan struct{}),
	}, nil
}

// armReadDeadline restarts the dead-connection timer; every ping and every
// successful read re-arms it.
func armReadDeadline(conn *websocket.Conn) {
	_ = conn.SetReadDeadline(time.Now().Add(reconnectPongWait))
}

// writePump is the sole goroutine that writes to the WebSocket connection.
// It ensures gorilla/websocket's contract of a single concurrent writer is maintained.
// writePump never closes conn.done; main() owns the done lifecycle.
func writePump(conn *Connection) {
	defer conn.conn.Close()

	for {
		select {
		case msg, ok := <-conn.Send:
			if !ok {
				return
			}

			err := conn.conn.WriteJSON(msg)
			if err != nil {
				log.Println("writePump error:", err)
				return
			}

		case <-conn.done:
			return
		}
	}
}

func waitForMessage(conn *Connection) tea.Cmd {
	return func() tea.Msg {
		// If there is a buffered first message, return it first
		if conn.firstMsg != nil {
			msg := IncomingMessage(*conn.firstMsg)
			conn.firstMsg = nil
			return msg
		}

		var msg IncomingMessage

		err := conn.conn.ReadJSON(&msg)
		if err != nil {
			return connErrMsg{err: err}
		}

		armReadDeadline(conn.conn)

		return msg
	}
}

// reconnectCmd redials with exponential backoff and rejoins with the
// session credentials. Dial failures retry indefinitely; a server join
// rejection is terminal.
func reconnectCmd(serverURL, room, nick, password, color string) tea.Cmd {
	return func() tea.Msg {
		backoff := time.Second

		for {
			conn, err := connectWebSocket(serverURL)
			if err == nil {
				msg, retry := rejoin(conn, room, nick, password, color)
				if !retry {
					return msg
				}

				close(conn.done)
				conn.conn.Close()
			}

			time.Sleep(backoff)
			backoff = nextBackoff(backoff)
		}
	}
}

func nextBackoff(d time.Duration) time.Duration {
	return min(d*2, maxReconnectBackoff)
}

// rejoin sends join (plus color) on a fresh socket and classifies the
// first server frame. retry is true only for transient read failures; a
// join rejection is fatal.
func rejoin(conn *Connection, room, nick, password, color string) (tea.Msg, bool) {
	go writePump(conn)

	conn.Send <- Message{
		Type:     "join",
		Nick:     nick,
		Room:     room,
		Password: password,
	}

	if color != "" {
		conn.Send <- Message{
			Type:  "color",
			Color: color,
		}
	}

	var first Message

	err := conn.conn.ReadJSON(&first)
	if err != nil {
		return nil, true
	}

	if first.Type == "error" {
		text := first.Text
		if text == "" {
			text = "rejoin rejected"
		}

		// The caller only tears down on retry; a fatal join owns it here.
		close(conn.done)
		conn.conn.Close()

		return connFatalMsg{err: errors.New(text)}, false
	}

	conn.firstMsg = &first
	conn.password = password

	return reconnectedMsg{conn: conn}, false
}

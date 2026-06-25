package main

import (
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

type Connection struct {
	conn     *websocket.Conn
	Send     chan Message          // buffered channel for writes
	firstMsg *Message              // buffered first message (used after password check)
	done     chan struct{}          // signal to stop writePump
}

func connectWebSocket(server string) (*Connection, error) {
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, err
	}

	return &Connection{
		conn: conn,
		Send: make(chan Message, 32),
		done: make(chan struct{}),
	}, nil
}

// writePump is the sole goroutine that writes to the WebSocket connection.
// It ensures gorilla/websocket's contract of a single concurrent writer is maintained.
// writePump never closes conn.done — main() owns the done lifecycle.
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
			return tea.Quit()
		}

		return msg
	}
}

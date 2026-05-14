package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

type Connection struct {
	conn *websocket.Conn
}

func connectWebSocket(server string) (*Connection, error) {
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, err
	}

	return &Connection{
		conn: conn,
	}, nil
}

func waitForMessage(conn *Connection) tea.Cmd {
	return func() tea.Msg {
		var msg IncomingMessage

		err := conn.conn.ReadJSON(&msg)
		if err != nil {
			return tea.Quit()
		}

		return msg
	}
}

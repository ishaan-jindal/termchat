package main

import (
	"log"

	"github.com/gorilla/websocket"
)

func connectWebSocket(server string) (*websocket.Conn, error) {
	conn, _, err := websocket.DefaultDialer.Dial(server, nil)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func readMessages(conn *websocket.Conn) {
	for {
		var msg Message

		err := conn.ReadJSON(&msg)
		if err != nil {
			log.Println("disconnected from server")
			return
		}

		switch msg.Type {
		case "system":
			println("[system]", msg.Text)

		case "message":
			println(msg.Nick+":", msg.Text)
		}
	}
}

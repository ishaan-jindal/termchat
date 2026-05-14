package main

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

const (
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(pongWait))

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	client := &Client{
		Conn: conn,
		Send: make(chan Message, 32),
	}

	// First message MUST be join message
	var joinMsg Message

	err = conn.ReadJSON(&joinMsg)
	if err != nil {
		log.Println(err)
		conn.Close()
		return
	}

	if joinMsg.Type != "join" {
		conn.Close()
		return
	}

	client.Nickname = joinMsg.Nick
	client.RoomID = joinMsg.Room

	room, exists := rooms[client.RoomID]

	if !exists {
		room = &Room{
			ID:      client.RoomID,
			Clients: make(map[*Client]bool),
		}

		rooms[client.RoomID] = room
	}

	room.Mutex.Lock()
	room.Clients[client] = true
	room.Mutex.Unlock()

	log.Printf("%s joined room %s\n", client.Nickname, client.RoomID)

	// Start writer FIRST
	go writePump(client)

	// Broadcast join event
	broadcastToRoom(client.RoomID, Message{
		Type: "system",
		Text: client.Nickname + " joined the room",
	})

	// Start reader loop
	readPump(client)
}

func readPump(client *Client) {
	defer cleanupClient(client)

	for {
		var msg Message

		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			log.Println(err)
			return
		}

		msg.Nick = client.Nickname

		broadcastToRoom(client.RoomID, msg)
	}
}

func writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)

	defer func() {
		ticker.Stop()
		client.Conn.Close()
	}()

	for {
		select {

		case msg, ok := <-client.Send:
			if !ok {
				return
			}

			err := client.Conn.WriteJSON(msg)
			if err != nil {
				log.Println(err)
				return
			}

		case <-ticker.C:
			err := client.Conn.WriteMessage(
				websocket.PingMessage,
				nil,
			)
			if err != nil {
				return
			}
		}
	}
}

func broadcastToRoom(roomID string, msg Message) {
	room, exists := rooms[roomID]

	if !exists {
		return
	}

	room.Mutex.Lock()

	clients := make([]*Client, 0, len(room.Clients))

	for client := range room.Clients {
		clients = append(clients, client)
	}

	room.Mutex.Unlock()

	for _, client := range clients {
		select {
		case client.Send <- msg:
		default:
			log.Println("dropping message for slow client")
		}
	}
}

func cleanupClient(client *Client) {
	room, exists := rooms[client.RoomID]

	if exists {
		// Broadcast leave message BEFORE removal
		broadcastToRoom(client.RoomID, Message{
			Type: "system",
			Text: client.Nickname + " left the room",
		})

		room.Mutex.Lock()

		delete(room.Clients, client)

		empty := len(room.Clients) == 0

		room.Mutex.Unlock()

		if empty {
			delete(rooms, room.ID)
		}
	}

	close(client.Send)
	client.Conn.Close()

	log.Printf("%s disconnected\n", client.Nickname)
}

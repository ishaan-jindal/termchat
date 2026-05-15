package main

import (
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

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

const (
	maxMessageLength = 500
	maxNickLength    = 32
)

const maxHistoryMessages = 30

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

	client.Nickname = sanitizeInput(joinMsg.Nick)
	if client.Nickname == "" {
		client.Nickname = "anonymous"
	}
	if len(client.Nickname) > maxNickLength {
		client.Nickname = client.Nickname[:maxNickLength]
	}
	client.Color = defaultColorForNick(client.Nickname)
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
	history := make([]Message, len(room.History))
	copy(history, room.History)
	room.Mutex.Unlock()

	log.Printf("%s joined room %s\n", client.Nickname, client.RoomID)

	// Start writer FIRST
	go writePump(client)

	client.Send <- Message{
		Type:     "history",
		Messages: history,
	}

	// Broadcast join event
	broadcastToRoom(client.RoomID, Message{
		Type: "system",
		Text: client.Nickname + " joined the room",
	})

	// Update User list
	broadcastUsersList(client.RoomID)

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

		msg.Text = sanitizeInput(msg.Text)
		msg.NewNick = sanitizeInput(msg.NewNick)

		if len(msg.Text) > maxMessageLength {
			msg.Text = msg.Text[:maxMessageLength]
		}

		if len(msg.NewNick) > maxNickLength {
			msg.NewNick = msg.NewNick[:maxNickLength]
		}

		if msg.Type == "nick" {
			oldNick := client.Nickname

			client.Nickname = sanitizeInput(msg.NewNick)
			if len(client.Nickname) > maxNickLength {
				client.Nickname = client.Nickname[:maxNickLength]
			}
			if client.Nickname == "" {
				client.Nickname = "anonymous"
			}
			client.Color = defaultColorForNick(client.Nickname)

			broadcastToRoom(client.RoomID, Message{
				Type: "system",
				Text: oldNick + " is now known as " + client.Nickname,
			})

			broadcastUsersList(client.RoomID)

			continue
		}

		if msg.Type == "users" {
			room := rooms[client.RoomID]

			room.Mutex.Lock()

			var users []string

			for c := range room.Clients {
				users = append(users, c.Nickname)
			}

			room.Mutex.Unlock()

			client.Send <- Message{
				Type: "users_list",
				Text: strings.Join(users, ", "),
			}

			continue
		}

		if msg.Type == "color" {
			if !isValidHexColor(msg.Color) {
				continue
			}
			client.Color = msg.Color

			client.Send <- Message{
				Type: "system",
				Text: "Color updated to " + client.Color,
			}

			continue
		}

		if msg.Type == "message" && msg.Text == "" {
			continue
		}

		msg.Nick = client.Nickname
		msg.Color = client.Color

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

	if msg.Type == "message" || msg.Type == "system" {
		room.History = append(room.History, msg)

		if len(room.History) > maxHistoryMessages {
			room.History = room.History[len(room.History)-maxHistoryMessages:]
		}
	}

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

	// Update User list
	broadcastUsersList(client.RoomID)

	log.Printf("%s disconnected\n", client.Nickname)
}

func broadcastUsersList(roomID string) {
	room, exists := rooms[roomID]

	if !exists {
		return
	}

	room.Mutex.Lock()

	var users []string

	for client := range room.Clients {
		users = append(users, client.Nickname)
	}

	clients := make([]*Client, 0, len(room.Clients))

	for client := range room.Clients {
		clients = append(clients, client)
	}

	room.Mutex.Unlock()

	msg := Message{
		Type: "users_list",
		Text: strings.Join(users, ", "),
	}

	for _, client := range clients {
		select {
		case client.Send <- msg:
		default:
		}
	}
}

func defaultColorForNick(nick string) string {
	colors := []string{
		"#00d7ff",
		"#5fd700",
		"#87ff00",
		"#ffd700",
		"#ffaf00",
		"#ff8700",
		"#ff5f5f",
		"#ff00af",
		"#d75fff",
		"#875fff",
		"#5f87ff",
		"#00afff",
		"#00ffd7",
		"#5fffaf",
		"#afff5f",
		"#ffff5f",
	}

	hash := 5381

	for _, c := range nick {
		hash = ((hash << 5) + hash) + int(c)
	}

	return colors[hash%len(colors)]
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

func sanitizeInput(input string) string {
	// remove ANSI escape sequences
	input = ansiRegex.ReplaceAllString(input, "")

	// remove control characters
	input = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}

		return r
	}, input)

	return strings.TrimSpace(input)
}

func isValidHexColor(color string) bool {
	re := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	return re.MatchString(color)
}

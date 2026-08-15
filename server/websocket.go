package server

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"termchat/shared"

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

	maxMessageLength   = 500
	maxNickLength      = 32
	maxHistoryMessages = 30

	maxMessagesPerSecond = 5
	idleTimeout          = 30 * time.Minute
)

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Println(err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(pongWait))

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	client := newClient(conn)

	// First message MUST be join message
	var joinMsg Message

	err = conn.ReadJSON(&joinMsg)
	if err != nil {
		logger.Println(err)
		conn.Close()
		return
	}

	if joinMsg.Type != "join" {
		conn.Close()
		return
	}

	client.mu.Lock()
	client.Nickname = truncateRunes(sanitizeInput(joinMsg.Nick), maxNickLength)
	if client.Nickname == "" {
		client.Nickname = "anonymous"
	}
	client.Color = defaultColorForNick(client.Nickname)
	client.mu.Unlock()

	client.RoomID = shared.NormalizeRoomCode(joinMsg.Room)
	if !shared.IsValidRoomCode(client.RoomID) {
		conn.Close()
		return
	}

	roomsMutex.Lock()
	room, exists := rooms[client.RoomID]

	if !exists {
		room = &Room{
			ID:       client.RoomID,
			Password: initialPassword,
			Clients:  make(map[*Client]bool),
		}

		rooms[client.RoomID] = room
	}
	roomsMutex.Unlock()

	// Password check
	room.Mutex.Lock()
	if room.Password != "" && joinMsg.Password != room.Password {
		room.Mutex.Unlock()
		conn.WriteJSON(Message{
			Type: "error",
			Text: "invalid_password",
		})
		conn.Close()
		return
	}

	room.Clients[client] = true

	// First client becomes host
	if room.Host == nil {
		room.Host = client
	}

	history := make([]Message, len(room.History))
	copy(history, room.History)
	room.Mutex.Unlock()

	client.mu.Lock()
	logger.Printf("%s joined room %s\n", client.Nickname, client.RoomID)
	client.mu.Unlock()

	// Start writer FIRST
	go writePump(client)

	client.trySend(Message{
		Type:     "history",
		Messages: history,
	})

	// Broadcast join event
	broadcastToRoom(client.RoomID, Message{
		Type: "system",
		Text: client.nickname() + " joined the room",
	})

	// Update User list
	broadcastUsersList(client.RoomID)

	// Start reader loop
	readPump(client)
}

func (c *Client) nickname() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.Nickname
}

func (c *Client) color() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.Color
}

func readPump(client *Client) {
	defer cleanupClient(client)

	for {
		var msg Message

		err := client.Conn.ReadJSON(&msg)
		if err != nil {
			logger.Println(err)
			return
		}

		// Update last activity
		client.mu.Lock()
		client.LastActivity = time.Now()
		client.mu.Unlock()

		now := time.Now()
		filtered := []time.Time{}
		for _, t := range client.MessageTimestamps {
			if now.Sub(t) < time.Second {
				filtered = append(filtered, t)
			}
		}
		client.MessageTimestamps = filtered
		if len(client.MessageTimestamps) >= maxMessagesPerSecond {
			continue
		}
		client.MessageTimestamps = append(
			client.MessageTimestamps,
			now,
		)

		msg.Text = truncateRunes(sanitizeInput(msg.Text), maxMessageLength)
		msg.NewNick = truncateRunes(sanitizeInput(msg.NewNick), maxNickLength)

		if msg.Type == "nick" {
			client.mu.Lock()
			oldNick := client.Nickname
			client.Nickname = msg.NewNick
			if client.Nickname == "" {
				client.Nickname = "anonymous"
			}
			client.Color = defaultColorForNick(client.Nickname)
			client.mu.Unlock()

			broadcastToRoom(client.RoomID, Message{
				Type: "system",
				Text: oldNick + " is now known as " + client.nickname(),
			})

			broadcastUsersList(client.RoomID)

			continue
		}

		if msg.Type == "color" {
			if !shared.IsValidHexColor(msg.Color) {
				continue
			}

			client.mu.Lock()
			client.Color = msg.Color
			client.mu.Unlock()

			broadcastUsersList(client.RoomID)

			client.trySend(Message{
				Type: "system",
				Text: "Color updated to " + client.color(),
			})

			continue
		}

		if msg.Type == "set_password" {
			roomsMutex.RLock()
			room := rooms[client.RoomID]
			roomsMutex.RUnlock()

			if room == nil {
				continue
			}

			room.Mutex.Lock()
			isHost := room.Host == client
			room.Mutex.Unlock()

			if !isHost {
				client.trySend(Message{
					Type: "system",
					Text: "Only the host can change the password",
				})
				continue
			}

			newPass := strings.TrimSpace(msg.Password)
			room.Mutex.Lock()
			room.Password = newPass
			room.Mutex.Unlock()

			if newPass == "" {
				broadcastToRoom(client.RoomID, Message{
					Type: "system",
					Text: "Room password removed — room is now unlocked",
				})
			} else {
				broadcastToRoom(client.RoomID, Message{
					Type: "system",
					Text: "Room password updated by host",
				})
			}

			continue
		}

		if msg.Type == "typing" {
			client.mu.Lock()
			wasTyping := client.Typing
			client.Typing = true
			client.LastTyping = time.Now()
			client.mu.Unlock()

			if !wasTyping {
				broadcastUsersList(client.RoomID)
			}

			continue
		}

		if msg.Type == "message" && msg.Text == "" {
			continue
		}

		if msg.Type != "message" {
			continue
		}

		client.mu.Lock()
		if client.Typing {
			client.Typing = false
			client.mu.Unlock()
			broadcastUsersList(client.RoomID)
		} else {
			client.mu.Unlock()
		}

		msg.Nick = client.nickname()
		msg.Color = client.color()

		broadcastToRoom(client.RoomID, msg)
	}
}

func writePump(client *Client) {
	ticker := time.NewTicker(pingPeriod)

	defer ticker.Stop()

	for {
		select {

		case msg, ok := <-client.Send:
			if !ok {
				return
			}

			err := client.Conn.WriteJSON(msg)
			if err != nil {
				logger.Println(err)
				return
			}

		case <-client.isDone():
			return

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
	roomsMutex.RLock()
	room, exists := rooms[roomID]
	roomsMutex.RUnlock()

	if !exists {
		return
	}

	room.Mutex.Lock()

	msg.Timestamp = time.Now().UnixMilli()

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
		client.trySend(msg)
	}
}

func cleanupClient(client *Client) {
	// Signal writePump and all broadcasters to stop targeting this client.
	client.close()

	roomsMutex.RLock()
	room, exists := rooms[client.RoomID]
	roomsMutex.RUnlock()

	if exists {
		room.Mutex.Lock()

		// Remove client from room first so broadcasts don't try to write
		// to the disconnecting client's closed connection.
		delete(room.Clients, client)

		empty := len(room.Clients) == 0

		// Host transfer: if this client was the host, pick the next oldest
		var newHostNick string

		if room.Host == client && !empty {
			room.Host = nil
			for c := range room.Clients {
				if room.Host == nil || c.JoinedAt.Before(room.Host.JoinedAt) {
					room.Host = c
				}
			}

			if room.Host != nil {
				newHostNick = room.Host.nickname()
			}
		}

		room.Mutex.Unlock()

		// Broadcast now — client is no longer in the room, won't receive
		broadcastToRoom(client.RoomID, Message{
			Type: "system",
			Text: client.nickname() + " left the room",
		})

		if newHostNick != "" {
			broadcastToRoom(client.RoomID, Message{
				Type: "system",
				Text: newHostNick + " is now the host",
			})
		}

		if empty {
			roomsMutex.Lock()
			delete(rooms, room.ID)
			roomsMutex.Unlock()
		}
	}

	client.Conn.Close()

	// Update User list
	broadcastUsersList(client.RoomID)

	logger.Printf("%s disconnected\n", client.nickname())
}

func broadcastUsersList(roomID string) {
	roomsMutex.RLock()
	room, exists := rooms[roomID]
	roomsMutex.RUnlock()

	if !exists {
		return
	}

	room.Mutex.Lock()

	var users []UserInfo

	for client := range room.Clients {
		client.mu.Lock()
		users = append(users, UserInfo{
			Nick:     client.Nickname,
			Color:    client.Color,
			JoinedAt: client.JoinedAt.Unix(),
			Typing:   client.Typing,
			IsHost:   room.Host == client,
		})
		client.mu.Unlock()
	}

	clients := make([]*Client, 0, len(room.Clients))

	for client := range room.Clients {
		clients = append(clients, client)
	}

	room.Mutex.Unlock()

	msg := Message{
		Type:  "users_list",
		Users: users,
	}

	for _, client := range clients {
		client.trySend(msg)
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

// truncateRunes limits s to at most max runes, never splitting a UTF-8
// sequence.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(s)

	if len(runes) <= max {
		return s
	}

	return string(runes[:max])
}

func cleanupIdleClients() {
	ticker := time.NewTicker(1 * time.Minute)

	defer ticker.Stop()

	for range ticker.C {
		roomsMutex.RLock()
		roomsCopy := make([]*Room, 0, len(rooms))
		for _, room := range rooms {
			roomsCopy = append(roomsCopy, room)
		}
		roomsMutex.RUnlock()

		for _, room := range roomsCopy {

			room.Mutex.Lock()

			clients := make([]*Client, 0, len(room.Clients))

			for client := range room.Clients {
				clients = append(clients, client)
			}

			room.Mutex.Unlock()

			for _, client := range clients {
				client.mu.Lock()
				idle := time.Since(client.LastActivity) > idleTimeout
				nick := client.Nickname
				client.mu.Unlock()

				if idle {
					logger.Printf(
						"disconnecting idle client %s",
						nick,
					)

					client.Conn.Close()
				}
			}
		}
	}
}

func cleanupTypingIndicators() {
	ticker := time.NewTicker(time.Second)

	defer ticker.Stop()

	for range ticker.C {
		roomsMutex.RLock()
		roomsCopy := make([]*Room, 0, len(rooms))
		for _, room := range rooms {
			roomsCopy = append(roomsCopy, room)
		}
		roomsMutex.RUnlock()

		for _, room := range roomsCopy {

			room.Mutex.Lock()

			changed := false

			for client := range room.Clients {
				client.mu.Lock()
				if client.Typing &&
					time.Since(client.LastTyping) > 3*time.Second {

					client.Typing = false
					changed = true
				}
				client.mu.Unlock()
			}

			roomID := room.ID

			room.Mutex.Unlock()

			if changed {
				broadcastUsersList(roomID)
			}
		}
	}
}

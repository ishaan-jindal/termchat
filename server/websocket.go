package server

import (
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
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

const maxMessageLength = 500

// The following limits are variables so tests can shrink the time scales.
var (
	pongWait             = 60 * time.Second
	pingPeriod           = (pongWait * 9) / 10
	maxHistoryMessages   = 30
	maxMessagesPerSecond = 5
	idleTimeout          = 30 * time.Minute
	typingExpiry         = 3 * time.Second
)

var livePumps atomic.Int64

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Println(err)
		return
	}

	conn.SetReadDeadline(time.Now().Add(pongWait))

	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))

		// Gorilla has no default read limit; cap frame size so a misbehaving
		// client cannot buffer unbounded messages.
		conn.SetReadLimit(4096)
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
	client.Nickname = sanitizeInput(joinMsg.Nick)
	if client.Nickname == "" {
		client.Nickname = "anonymous"
	}

	if !shared.IsValidNickname(client.Nickname) {
		client.mu.Unlock()
		conn.WriteJSON(Message{
			Type: "error",
			Text: "invalid_nick",
		})
		conn.Close()
		return
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
			ID:        client.RoomID,
			Password:  initialPassword,
			Clients:   make(map[*Client]bool),
			NextID:    1,
			Reactions: make(map[int64]map[string]map[*Client]bool),
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
	livePumps.Add(1)
	defer livePumps.Add(-1)

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
		msg.NewNick = sanitizeInput(msg.NewNick)

		if msg.Type == "nick" {
			if !shared.IsValidNickname(msg.NewNick) {
				client.trySend(Message{
					Type: "error",
					Text: "invalid_nick",
				})
				continue
			}

			client.mu.Lock()
			oldNick := client.Nickname
			client.Nickname = msg.NewNick
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
					Text: "Room password removed - room is now unlocked",
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

		if msg.Type == "reaction" {
			handleReaction(client, msg)
			continue
		}

		if msg.Type == "users" {
			list, _, ok := usersSnapshot(client.RoomID)
			if ok {
				client.trySend(list)
			}

			continue
		}

		if msg.Type == "message" && msg.Text == "" {
			continue
		}

		if msg.Type != "message" {
			continue
		}

		// Resolve the reply quote from history; the server is the authority
		// on quoted text. Unknown targets send the message plain.
		if msg.ReplyToID != 0 {
			roomsMutex.RLock()
			room, exists := rooms[client.RoomID]
			roomsMutex.RUnlock()

			found := false

			if exists {
				room.Mutex.Lock()

				for i := range room.History {
					histMsg := &room.History[i]

					if histMsg.ID == msg.ReplyToID && histMsg.Type == "message" {
						msg.ReplyToNick = histMsg.Nick
						msg.ReplyToText = histMsg.Text
						found = true

						break
					}
				}

				room.Mutex.Unlock()
			}

			if !found {
				msg.ReplyToID = 0
			}
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
	livePumps.Add(1)
	defer livePumps.Add(-1)

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
		msg.ID = room.NextID
		room.NextID++
		room.History = append(room.History, msg)

		if len(room.History) > maxHistoryMessages {
			dropped := room.History[:len(room.History)-maxHistoryMessages]
			room.History = room.History[len(room.History)-maxHistoryMessages:]

			for _, m := range dropped {
				delete(room.Reactions, m.ID)
			}
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

// handleReaction toggles the sender's vote on a chat message and broadcasts
// the updated counts. Invalid names and unknown targets are ignored.
func handleReaction(client *Client, msg Message) {
	if !shared.IsValidReaction(msg.Text) || msg.ID <= 0 {
		return
	}

	roomsMutex.RLock()
	room, exists := rooms[client.RoomID]
	roomsMutex.RUnlock()

	if !exists {
		return
	}

	room.Mutex.Lock()

	targetIdx := -1

	for i := range room.History {
		histMsg := &room.History[i]

		if histMsg.ID == msg.ID && histMsg.Type == "message" {
			targetIdx = i

			break
		}
	}

	if targetIdx == -1 {
		room.Mutex.Unlock()

		return
	}

	voters, ok := room.Reactions[msg.ID]

	if !ok {
		voters = make(map[string]map[*Client]bool)
		room.Reactions[msg.ID] = voters
	}

	names, ok := voters[msg.Text]

	if !ok {
		names = make(map[*Client]bool)
		voters[msg.Text] = names
	}

	if names[client] {
		delete(names, client)
	} else {
		names[client] = true
	}

	if len(names) == 0 {
		delete(voters, msg.Text)
	}

	if len(voters) == 0 {
		delete(room.Reactions, msg.ID)
	}

	// Iterate the allowlist so broadcast order is deterministic.
	var reactions []shared.Reaction

	for _, name := range shared.ReactionNames {
		if set, ok := voters[name]; ok {
			reactions = append(reactions, shared.Reaction{
				Name:  name,
				Count: len(set),
			})
		}
	}

	// Store counts on the history message so replay carries them.
	room.History[targetIdx].Reactions = reactions

	room.Mutex.Unlock()

	broadcastToRoom(client.RoomID, Message{
		Type:      "reaction",
		ID:        msg.ID,
		Nick:      client.nickname(),
		Reactions: reactions,
	})
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

		// Broadcast now; client is no longer in the room, so it will not receive
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

// usersSnapshot builds a room's users_list frame together with the clients it
// should go to. The bool is false when the room no longer exists.
func usersSnapshot(roomID string) (Message, []*Client, bool) {
	roomsMutex.RLock()
	room, exists := rooms[roomID]
	roomsMutex.RUnlock()

	if !exists {
		return Message{}, nil, false
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
		Type:       "users_list",
		ServerTime: time.Now().Unix(),
		Users:      users,
	}

	return msg, clients, true
}

func broadcastUsersList(roomID string) {
	msg, clients, ok := usersSnapshot(roomID)
	if !ok {
		return
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
// sequence. Malformed input is normalized to replacement characters so the
// result is always valid UTF-8.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}

	runes := []rune(s)

	// Re-encoding the rune slice normalizes invalid byte sequences to
	// U+FFFD, guaranteeing valid UTF-8 output for any input.
	if len(runes) <= max {
		return string(runes)
	}

	return string(runes[:max])
}

func cleanupIdleClients(stop <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)

	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cleanupIdleClientsOnce()
		}
	}
}

func cleanupIdleClientsOnce() {
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

func cleanupTypingIndicators(stop <-chan struct{}) {
	ticker := time.NewTicker(time.Second)

	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cleanupTypingIndicatorsOnce()
		}
	}
}

func cleanupTypingIndicatorsOnce() {
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
				time.Since(client.LastTyping) > typingExpiry {

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

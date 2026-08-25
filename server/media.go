package server

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"termchat/shared"

	"github.com/gorilla/websocket"
)

var (
	mediaTokenTTL = 30 * time.Second

	mediaPongWait            = 30 * time.Second
	mediaPingPeriod          = 10 * time.Second
	maxMediaFrameSize        = int64(16384)
	mediaUpstreamBytesPerSec = 128 * 1024
	mediaSendBuffer          = 64
)

const mediaTokenBytes = 16

type mediaTokenEntry struct {
	client    *Client
	expiresAt time.Time
}

var mediaTokens = struct {
	sync.Mutex
	tokens map[string]mediaTokenEntry
}{tokens: map[string]mediaTokenEntry{}}

func resetMediaTokens() {
	mediaTokens.Lock()
	mediaTokens.tokens = map[string]mediaTokenEntry{}
	mediaTokens.Unlock()
}

// issueMediaToken mints a single-use token bound to the client.
func issueMediaToken(client *Client) string {
	buf := make([]byte, mediaTokenBytes)

	if _, err := rand.Read(buf); err != nil {
		logger.Println("issuing media token:", err)
		return ""
	}

	token := hex.EncodeToString(buf)

	mediaTokens.Lock()
	mediaTokens.tokens[token] = mediaTokenEntry{
		client:    client,
		expiresAt: time.Now().Add(mediaTokenTTL),
	}
	mediaTokens.Unlock()

	return token
}

// consumeMediaToken redeems a token exactly once, returning its client or
// nil when the token is unknown, expired or already spent.
func consumeMediaToken(token string) *Client {
	mediaTokens.Lock()
	defer mediaTokens.Unlock()

	entry, ok := mediaTokens.tokens[token]

	if ok {
		delete(mediaTokens.tokens, token)
	}

	if !ok || time.Now().After(entry.expiresAt) {
		return nil
	}

	return entry.client
}

type mediaConn struct {
	client    *Client
	conn      *websocket.Conn
	send      chan []byte
	done      chan struct{}
	closeOnce sync.Once
	voiceID   uint32
}

func (m *mediaConn) close() {
	m.closeOnce.Do(func() {
		close(m.done)
		m.conn.Close()
	})
}

func (m *mediaConn) trySend(frame []byte) bool {
	select {
	case m.send <- frame:
		return true
	case <-m.done:
		return false
	default:
		return false
	}
}

// mediaHub tracks active voice connections per client. Its mutex is a leaf:
// never hold it while acquiring room or client locks.
type mediaHub struct {
	mu    sync.Mutex
	conns map[*Client]*mediaConn
	voice map[uint32]*Client
}

var hub = &mediaHub{
	conns: map[*Client]*mediaConn{},
	voice: map[uint32]*Client{},
}

func resetMediaHub() {
	hub.mu.Lock()
	hub.conns = map[*Client]*mediaConn{}
	hub.voice = map[uint32]*Client{}
	hub.mu.Unlock()
}

func randomUint32() uint32 {
	var buf [4]byte

	if _, err := rand.Read(buf[:]); err != nil {
		return 0
	}

	return binary.BigEndian.Uint32(buf[:])
}

// register admits a client's voice connection and assigns a unique nonzero
// voice ID. It reports false when the client already has a voice session or
// no unique ID could be generated.
func (h *mediaHub) register(client *Client, conn *websocket.Conn) (*mediaConn, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.conns[client]; exists {
		return nil, false
	}

	for attempt := 0; attempt < 5; attempt++ {
		id := randomUint32()

		if id == 0 {
			continue
		}

		if _, taken := h.voice[id]; taken {
			continue
		}

		mc := &mediaConn{
			client:  client,
			conn:    conn,
			send:    make(chan []byte, mediaSendBuffer),
			done:    make(chan struct{}),
			voiceID: id,
		}
		h.conns[client] = mc
		h.voice[id] = client

		return mc, true
	}

	return nil, false
}

// remove detaches the client's voice session if present; it is silent so
// callers decide when to broadcast the cleared voice flag.
func (h *mediaHub) remove(client *Client) {
	h.mu.Lock()

	mc, ok := h.conns[client]

	if ok {
		delete(h.conns, client)
		delete(h.voice, mc.voiceID)
	}

	h.mu.Unlock()

	if !ok {
		return
	}

	mc.close()

	client.mu.Lock()
	client.VoiceID = 0
	client.mu.Unlock()
}

func (h *mediaHub) closeAll() {
	h.mu.Lock()
	conns := make([]*mediaConn, 0, len(h.conns))

	for _, mc := range h.conns {
		conns = append(conns, mc)
	}
	h.mu.Unlock()

	for _, mc := range conns {
		mc.close()
	}
}

// relay fans a stamped frame out to the sender's room peers. Lock order is
// roomsMutex -> room.Mutex, released before the hub lookup.
func (h *mediaHub) relay(sender *Client, frame []byte) {
	roomsMutex.RLock()
	room, exists := rooms[sender.RoomID]
	roomsMutex.RUnlock()

	if !exists {
		return
	}

	room.Mutex.Lock()

	peers := make([]*Client, 0, len(room.Clients))
	for c := range room.Clients {
		if c != sender {
			peers = append(peers, c)
		}
	}

	room.Mutex.Unlock()

	h.mu.Lock()
	targets := make([]*mediaConn, 0, len(peers))

	for _, p := range peers {
		if mc, ok := h.conns[p]; ok {
			targets = append(targets, mc)
		}
	}

	h.mu.Unlock()

	for _, mc := range targets {
		mc.trySend(frame)
	}
}

// rejectMedia answers a media join with an error frame and closes the conn.
func rejectMedia(conn *websocket.Conn, text string) {
	conn.WriteJSON(Message{Type: "error", Text: text})
	conn.Close()
}

func handleMediaWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logger.Println(err)
		return
	}

	var join Message

	if err := conn.ReadJSON(&join); err != nil || join.Type != "join" || join.Token == "" {
		conn.Close()
		return
	}

	// The token is burned on first read even when validation fails, so a
	// captured token cannot be retried against other validations.
	client := consumeMediaToken(join.Token)

	if client == nil {
		rejectMedia(conn, "invalid_token")
		return
	}

	select {
	case <-client.isDone():
		rejectMedia(conn, "invalid_token")
		return
	default:
	}

	roomID := shared.NormalizeRoomCode(join.Room)

	if roomID != client.RoomID {
		rejectMedia(conn, "invalid_token")
		return
	}

	roomsMutex.RLock()
	room, exists := rooms[client.RoomID]
	roomsMutex.RUnlock()

	member := false

	if exists {
		room.Mutex.Lock()
		member = room.Clients[client]
		room.Mutex.Unlock()
	}

	if !member {
		rejectMedia(conn, "invalid_token")
		return
	}

	mc, ok := hub.register(client, conn)

	if !ok {
		rejectMedia(conn, "already_in_voice")
		return
	}

	client.mu.Lock()
	client.VoiceID = mc.voiceID
	client.mu.Unlock()

	conn.SetReadLimit(maxMediaFrameSize)
	conn.SetReadDeadline(time.Now().Add(mediaPongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(mediaPongWait))
		return nil
	})

	broadcastUsersList(client.RoomID)

	// The ack goes out before the write pump starts so this conn keeps
	// gorilla's single-writer guarantee.
	conn.WriteJSON(Message{Type: "ok"})

	go mediaWritePump(mc)

	defer func() {
		hub.remove(client)
		broadcastUsersList(client.RoomID)
	}()

	mediaReadPump(mc)
}

// mediaReadPump validates inbound frames, stamps the sender's voice ID over
// whatever the client wrote there, and relays to room peers. It runs on the
// handler goroutine; its exit tears the voice session down via defer.
func mediaReadPump(mc *mediaConn) {
	livePumps.Add(1)
	defer livePumps.Add(-1)

	budget := int64(mediaUpstreamBytesPerSec)
	lastRefill := time.Now()

	for {
		msgType, frame, err := mc.conn.ReadMessage()

		if err != nil {
			return
		}

		now := time.Now()
		budget += int64(now.Sub(lastRefill).Seconds() * float64(mediaUpstreamBytesPerSec))

		if budget > int64(mediaUpstreamBytesPerSec) {
			budget = int64(mediaUpstreamBytesPerSec)
		}

		lastRefill = now
		budget -= int64(len(frame))

		if budget < 0 || msgType != websocket.BinaryMessage {
			return
		}

		kind, _, _, _, ok := shared.ParseMediaFrame(frame)

		if !ok || kind != shared.MediaKindAudio {
			return
		}

		binary.BigEndian.PutUint32(frame[2:shared.MediaHeaderLen], mc.voiceID)

		hub.relay(mc.client, frame)
	}
}

func mediaWritePump(mc *mediaConn) {
	livePumps.Add(1)
	defer livePumps.Add(-1)

	ticker := time.NewTicker(mediaPingPeriod)
	defer ticker.Stop()

	for {
		select {
		case frame, ok := <-mc.send:
			if !ok {
				return
			}

			err := mc.conn.WriteMessage(websocket.BinaryMessage, frame)
			if err != nil {
				return
			}

		case <-mc.done:
			return

		case <-ticker.C:
			err := mc.conn.WriteMessage(websocket.PingMessage, nil)
			if err != nil {
				return
			}
		}
	}
}

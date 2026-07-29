package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSanitizeInputNormalText(t *testing.T) {
	got := sanitizeInput("hello world")
	if got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestSanitizeInputAnsiEscape(t *testing.T) {
	got := sanitizeInput("\x1b[31mred\x1b[0m")
	if got != "red" {
		t.Fatalf("got %q, want %q", got, "red")
	}
}

func TestSanitizeInputControlChars(t *testing.T) {
	got := sanitizeInput("hello\x00\x01\x02world")
	if got != "helloworld" {
		t.Fatalf("got %q, want %q", got, "helloworld")
	}
}

func TestSanitizeInputPreservesNewlineAndTab(t *testing.T) {
	got := sanitizeInput("hello\n\tworld")
	if got != "hello\n\tworld" {
		t.Fatalf("got %q, want %q", got, "hello\n\tworld")
	}
}

func TestSanitizeInputTrimsWhitespace(t *testing.T) {
	got := sanitizeInput("  hello  ")
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
}

func TestSanitizeInputEmpty(t *testing.T) {
	got := sanitizeInput("")
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestSanitizeInputOnlyWhitespace(t *testing.T) {
	got := sanitizeInput("   \t\n  ")
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestSanitizeInputUnicode(t *testing.T) {
	got := sanitizeInput("héllo 世界")
	if got != "héllo 世界" {
		t.Fatalf("got %q, want %q", got, "héllo 世界")
	}
}

func TestDefaultColorForNickDeterministic(t *testing.T) {
	c1 := defaultColorForNick("alice")
	c2 := defaultColorForNick("alice")
	if c1 != c2 {
		t.Fatalf("same nick produced different colors: %q vs %q", c1, c2)
	}
}

var validColors = map[string]bool{
	"#00d7ff": true,
	"#5fd700": true,
	"#87ff00": true,
	"#ffd700": true,
	"#ffaf00": true,
	"#ff8700": true,
	"#ff5f5f": true,
	"#ff00af": true,
	"#d75fff": true,
	"#875fff": true,
	"#5f87ff": true,
	"#00afff": true,
	"#00ffd7": true,
	"#5fffaf": true,
	"#afff5f": true,
	"#ffff5f": true,
}

func TestDefaultColorForNickValidColor(t *testing.T) {
	for _, nick := range []string{"alice", "bob", "test_user", "123", "!@#$"} {
		color := defaultColorForNick(nick)
		if !validColors[color] {
			t.Fatalf("nick %q produced invalid color %q", nick, color)
		}
	}
}

func TestDefaultColorForNickEmpty(t *testing.T) {
	color := defaultColorForNick("")
	if !validColors[color] {
		t.Fatalf("empty nick produced invalid color %q", color)
	}
}

func setupTestRooms(t *testing.T) func() {
	t.Helper()
	oldRooms := rooms
	rooms = map[string]*Room{}
	return func() {
		rooms = oldRooms
	}
}

func TestBroadcastToRoomMessageDelivery(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	c1 := &Client{Nickname: "a", Send: make(chan Message, 32)}
	c2 := &Client{Nickname: "b", Send: make(chan Message, 32)}

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Clients: map[*Client]bool{c1: true, c2: true},
	}
	roomsMutex.Unlock()

	broadcastToRoom("TEST", Message{Type: "message", Text: "hello"})

	for _, c := range []*Client{c1, c2} {
		select {
		case msg := <-c.Send:
			if msg.Text != "hello" {
				t.Fatalf("client %s got text %q, want hello", c.Nickname, msg.Text)
			}
			if msg.Timestamp == 0 {
				t.Fatal("expected non-zero timestamp")
			}
		case <-time.After(time.Second):
			t.Fatalf("client %s did not receive message", c.Nickname)
		}
	}
}

func TestBroadcastToRoomHistory(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	c := &Client{Send: make(chan Message, 32)}
	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Clients: map[*Client]bool{c: true},
	}
	roomsMutex.Unlock()

	broadcastToRoom("TEST", Message{Type: "message", Text: "msg1"})
	broadcastToRoom("TEST", Message{Type: "message", Text: "msg2"})

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	room.Mutex.Lock()
	histLen := len(room.History)
	room.Mutex.Unlock()

	if histLen != 2 {
		t.Fatalf("history len = %d, want 2", histLen)
	}
}

func TestBroadcastToRoomHistoryCap(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	c := &Client{Send: make(chan Message, 64)}
	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Clients: map[*Client]bool{c: true},
	}
	roomsMutex.Unlock()

	for i := 0; i < maxHistoryMessages+5; i++ {
		broadcastToRoom("TEST", Message{Type: "message", Text: "msg"})
	}

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	room.Mutex.Lock()
	histLen := len(room.History)
	room.Mutex.Unlock()

	if histLen != maxHistoryMessages {
		t.Fatalf("history len = %d, want %d", histLen, maxHistoryMessages)
	}
}

func TestBroadcastToRoomSlowClient(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	slow := &Client{Send: make(chan Message)}
	fast := &Client{Send: make(chan Message, 32)}

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Clients: map[*Client]bool{slow: true, fast: true},
	}
	roomsMutex.Unlock()

	broadcastToRoom("TEST", Message{Type: "message", Text: "hello"})

	select {
	case msg := <-fast.Send:
		if msg.Text != "hello" {
			t.Fatalf("fast client got %q, want hello", msg.Text)
		}
	case <-time.After(time.Second):
		t.Fatal("fast client did not receive message")
	}
}

func TestBroadcastToRoomNonexistentRoom(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	broadcastToRoom("NOPE", Message{Type: "message", Text: "hello"})
}

func TestBroadcastToRoomNonMessageNotInHistory(t *testing.T) {
	cleanup := setupTestRooms(t)
	defer cleanup()

	c := &Client{Send: make(chan Message, 32)}
	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Clients: map[*Client]bool{c: true},
	}
	roomsMutex.Unlock()

	broadcastToRoom("TEST", Message{Type: "typing", Text: "..."})

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	room.Mutex.Lock()
	histLen := len(room.History)
	room.Mutex.Unlock()

	if histLen != 0 {
		t.Fatalf("non-message type should not be in history, got len %d", histLen)
	}
}

func newClientWithConn(t *testing.T, nick string, joinedAt time.Time) (*Client, *httptest.Server) {
	t.Helper()

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}))

	url := "ws" + strings.TrimPrefix(s.URL, "http") + "/ws"
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		s.Close()
		t.Fatalf("dial: %v", err)
	}

	return &Client{
		Conn:     ws,
		Nickname: nick,
		Send:     make(chan Message, 32),
		JoinedAt: joinedAt,
		RoomID:   "TEST",
	}, s
}

func newFakeClient(nick string, joinedAt time.Time) *Client {
	return &Client{
		Nickname: nick,
		Send:     make(chan Message, 32),
		JoinedAt: joinedAt,
		RoomID:   "TEST",
	}
}

func TestCleanupClientRemovesClient(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	member, memberSrv := newClientWithConn(t, "bob", time.Now())
	defer memberSrv.Close()

	host := newFakeClient("alice", time.Now().Add(-10*time.Second))

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    host,
		Clients: map[*Client]bool{host: true, member: true},
	}
	roomsMutex.Unlock()

	cleanupClient(member)

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	if room == nil {
		t.Fatal("room should still exist")
	}
	room.Mutex.Lock()
	_, stillInRoom := room.Clients[member]
	room.Mutex.Unlock()
	if stillInRoom {
		t.Fatal("member should be removed from room")
	}
}

func TestCleanupClientHostTransfer(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	host, hostSrv := newClientWithConn(t, "alice", time.Now().Add(-20*time.Second))
	defer hostSrv.Close()

	member := newFakeClient("bob", time.Now().Add(-10*time.Second))
	third := newFakeClient("carol", time.Now())

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    host,
		Clients: map[*Client]bool{host: true, member: true, third: true},
	}
	roomsMutex.Unlock()

	cleanupClient(host)

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	if room == nil {
		t.Fatal("room should still exist")
	}
	room.Mutex.Lock()
	newHost := room.Host
	room.Mutex.Unlock()

	if newHost == nil {
		t.Fatal("expected a new host")
	}
	if newHost != member {
		t.Fatalf("expected bob to be new host (oldest), got %s", newHost.Nickname)
	}
}

func TestCleanupClientEmptyRoomDeleted(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	client, srv := newClientWithConn(t, "lonely", time.Now())
	defer srv.Close()

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    client,
		Clients: map[*Client]bool{client: true},
	}
	roomsMutex.Unlock()

	cleanupClient(client)

	roomsMutex.RLock()
	_, exists := rooms["TEST"]
	roomsMutex.RUnlock()

	if exists {
		t.Fatal("empty room should be deleted")
	}
}

func TestCleanupClientBroadcastsLeaveMessage(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	leaver, leaverSrv := newClientWithConn(t, "alice", time.Now().Add(-10*time.Second))
	defer leaverSrv.Close()

	stayer := newFakeClient("bob", time.Now())

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    leaver,
		Clients: map[*Client]bool{leaver: true, stayer: true},
	}
	roomsMutex.Unlock()

	cleanupClient(leaver)

	close(stayer.Send)
	var msgs []Message
	for msg := range stayer.Send {
		msgs = append(msgs, msg)
	}

	foundLeave := false
	for _, msg := range msgs {
		if msg.Type == "system" && strings.Contains(msg.Text, "left the room") {
			foundLeave = true
			break
		}
	}
	if !foundLeave {
		t.Fatal("expected leave message to be broadcast")
	}
}

func TestCleanupClientBroadcastsHostTransfer(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	host, hostSrv := newClientWithConn(t, "alice", time.Now().Add(-10*time.Second))
	defer hostSrv.Close()

	member := newFakeClient("bob", time.Now())

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    host,
		Clients: map[*Client]bool{host: true, member: true},
	}
	roomsMutex.Unlock()

	cleanupClient(host)

	close(member.Send)
	var msgs []Message
	for msg := range member.Send {
		msgs = append(msgs, msg)
	}

	foundHost := false
	for _, msg := range msgs {
		if msg.Type == "system" && strings.Contains(msg.Text, "is now the host") {
			foundHost = true
			break
		}
	}
	if !foundHost {
		t.Fatal("expected host transfer message")
	}
}

func TestCleanupClientNoHostTransferWhenEmpty(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	client, srv := newClientWithConn(t, "only", time.Now())
	defer srv.Close()

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    client,
		Clients: map[*Client]bool{client: true},
	}
	roomsMutex.Unlock()

	cleanupClient(client)

	roomsMutex.RLock()
	_, exists := rooms["TEST"]
	roomsMutex.RUnlock()

	if exists {
		t.Fatal("empty room should be deleted")
	}
}

func TestCleanupClientNonHostLeave(t *testing.T) {
	oldRooms := rooms
	rooms = map[string]*Room{}
	defer func() { rooms = oldRooms }()

	host := newFakeClient("alice", time.Now().Add(-20*time.Second))
	member, memberSrv := newClientWithConn(t, "bob", time.Now().Add(-10*time.Second))
	defer memberSrv.Close()

	roomsMutex.Lock()
	rooms["TEST"] = &Room{
		ID:      "TEST",
		Host:    host,
		Clients: map[*Client]bool{host: true, member: true},
	}
	roomsMutex.Unlock()

	cleanupClient(member)

	roomsMutex.RLock()
	room := rooms["TEST"]
	roomsMutex.RUnlock()

	if room == nil {
		t.Fatal("room should still exist")
	}
	room.Mutex.Lock()
	stillHost := room.Host == host
	room.Mutex.Unlock()

	if !stillHost {
		t.Fatal("host should not change when non-host leaves")
	}
}

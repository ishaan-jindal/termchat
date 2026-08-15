package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"termchat/shared"

	"github.com/gorilla/websocket"
)

func TestMain(m *testing.M) {
	SetLogOutput(io.Discard)
	m.Run()
}

func resetState(t *testing.T) {
	t.Helper()

	roomsMutex.Lock()
	rooms = map[string]*Room{}
	roomsMutex.Unlock()

	initialPassword = ""
}

func startTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWebSocket)
	mux.HandleFunc("/discover", handleDiscover)

	srv := httptest.NewServer(mux)

	t.Cleanup(func() {
		srv.Close()
		resetState(t)
	})

	return srv
}

func wsURL(t *testing.T, srv *httptest.Server) string {
	t.Helper()

	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

type testClient struct {
	t    *testing.T
	conn *websocket.Conn
	msgs chan shared.Message
}

func joinRoom(t *testing.T, srv *httptest.Server, room, nick, password string) *testClient {
	t.Helper()

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	join := shared.Message{Type: "join", Nick: nick, Room: room}
	if password != "" {
		join.Password = password
	}

	if err := conn.WriteJSON(join); err != nil {
		t.Fatalf("join write: %v", err)
	}

	c := &testClient{
		t:    t,
		conn: conn,
		msgs: make(chan shared.Message, 128),
	}

	go c.readLoop()

	return c
}

func (c *testClient) readLoop() {
	for {
		var m shared.Message

		if err := c.conn.ReadJSON(&m); err != nil {
			close(c.msgs)
			return
		}

		c.msgs <- m
	}
}

func (c *testClient) next() (shared.Message, bool) {
	select {
	case m, ok := <-c.msgs:
		return m, ok
	case <-time.After(3 * time.Second):
		c.t.Fatalf("timed out waiting for message on %s", c.conn.RemoteAddr())
		return shared.Message{}, false
	}
}

func (c *testClient) nextOfType(typ string) shared.Message {
	c.t.Helper()

	for {
		m, ok := c.next()
		if !ok {
			c.t.Fatalf("connection closed while waiting for %q", typ)
		}

		if m.Type == typ {
			return m
		}
	}
}

func (c *testClient) send(m shared.Message) {
	c.t.Helper()

	c.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))

	if err := c.conn.WriteJSON(m); err != nil {
		c.t.Fatalf("write %s: %v", m.Type, err)
	}
}

func (c *testClient) close() {
	c.conn.Close()
}

func roomState(t *testing.T, roomID string) *Room {
	t.Helper()

	roomsMutex.RLock()
	defer roomsMutex.RUnlock()

	return rooms[roomID]
}

func TestJoinReceivesHistorySystemAndUsers(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "JOIN", "alice", "")
	defer a.close()

	a.nextOfType("history")

	if got := a.nextOfType("system").Text; got != "alice joined the room" {
		t.Errorf("system text = %q", got)
	}

	a.nextOfType("users_list")

	b := joinRoom(t, srv, "JOIN", "bob", "")
	defer b.close()

	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")

	users := a.nextOfType("users_list").Users

	if len(users) != 2 {
		t.Fatalf("users = %d, want 2", len(users))
	}

	seen := map[string]bool{}

	for _, u := range users {
		seen[u.Nick] = true
	}

	if !seen["alice"] || !seen["bob"] {
		t.Errorf("users missing participants: %v", seen)
	}
}

func TestMessageBroadcast(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "MSGX", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b := joinRoom(t, srv, "MSGX", "bob", "")
	defer b.close()

	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")

	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{Type: "message", Text: "hello world"})

	msg := a.nextOfType("message")

	if msg.Nick != "bob" {
		t.Errorf("nick = %q, want bob", msg.Nick)
	}

	if msg.Text != "hello world" {
		t.Errorf("text = %q, want %q", msg.Text, "hello world")
	}

	if msg.Timestamp == 0 {
		t.Error("timestamp not set")
	}

	if msg.Color == "" {
		t.Error("color not set")
	}

	// Room history must contain the message as the latest entry
	room := roomState(t, "MSGX")
	room.Mutex.Lock()
	hist := make([]Message, len(room.History))
	copy(hist, room.History)
	room.Mutex.Unlock()

	if len(hist) == 0 || hist[len(hist)-1].Text != "hello world" {
		t.Errorf("history does not end with the message: %+v", hist)
	}
}

func TestPasswordProtection(t *testing.T) {
	srv := startTestServer(t)

	host := joinRoom(t, srv, "LOCK", "host", "")
	defer host.close()

	host.nextOfType("history")
	host.nextOfType("system")
	host.nextOfType("users_list")

	host.send(shared.Message{Type: "set_password", Password: "secret"})
	host.nextOfType("system")

	// Wrong password: error message then disconnect
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}

	conn.WriteJSON(shared.Message{Type: "join", Nick: "evil", Room: "LOCK", Password: "nope"})

	var reply shared.Message

	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatalf("read error reply: %v", err)
	}

	if reply.Type != "error" || reply.Text != "invalid_password" {
		t.Errorf("reply = %+v, want invalid_password error", reply)
	}

	conn.SetReadDeadline(time.Now().Add(time.Second))
	if err := conn.ReadJSON(&reply); err == nil {
		t.Error("connection still open after wrong password")
	}
	conn.Close()

	// Correct password succeeds
	good := joinRoom(t, srv, "LOCK", "friend", "secret")
	defer good.close()

	good.nextOfType("history")
	good.nextOfType("system")
	good.nextOfType("users_list")

	// Non-host cannot change password
	good.send(shared.Message{Type: "set_password", Password: "hacked"})

	if got := good.nextOfType("system").Text; !strings.Contains(got, "Only the host") {
		t.Errorf("non-host password change reply = %q", got)
	}
}

func TestHostSuccession(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "HOST", "alice", "")
	b := joinRoom(t, srv, "HOST", "bob", "")

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")
	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	if host := hostOf(t, "HOST"); host != "alice" {
		t.Fatalf("host before disconnect = %q, want alice", host)
	}

	a.close()

	// bob should be told he left and that bob is now the host
	b.nextOfType("system")
	if got := b.nextOfType("system").Text; got != "bob is now the host" {
		t.Errorf("succession message = %q", got)
	}

	if host := hostOf(t, "HOST"); host != "bob" {
		t.Errorf("host after alice left = %q, want bob", host)
	}

	b.close()
}

func hostOf(t *testing.T, roomID string) string {
	t.Helper()

	room := roomState(t, roomID)
	if room == nil {
		return ""
	}

	room.Mutex.Lock()
	defer room.Mutex.Unlock()

	if room.Host == nil {
		return ""
	}

	return room.Host.nickname()
}

func TestRoomDeletedWhenEmpty(t *testing.T) {
	srv := startTestServer(t)

	c := joinRoom(t, srv, "DELR", "solo", "")
	c.nextOfType("history")
	c.nextOfType("system")
	c.nextOfType("users_list")
	c.close()

	deadline := time.Now().Add(2 * time.Second)

	for time.Now().Before(deadline) {
		if roomState(t, "DELR") == nil {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Error("room not deleted after last client left")
}

func TestMessageSanitization(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "SANI", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "SANI", "bob", "")
	defer b.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{
		Type: "message",
		Text: "hello \x1b[31mred\x1b[0m \x07bell",
	})

	msg := a.nextOfType("message")

	if strings.Contains(msg.Text, "\x1b") {
		t.Errorf("ANSI escape sequences survived sanitization: %q", msg.Text)
	}

	if strings.Contains(msg.Text, "\x07") {
		t.Errorf("control character survived sanitization: %q", msg.Text)
	}

	if msg.Text != "hello red bell" {
		t.Errorf("text = %q", msg.Text)
	}
}

func TestNickChange(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "NICK", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "NICK", "bob", "")
	defer b.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{Type: "nick", NewNick: "robert"})

	if got := a.nextOfType("system").Text; got != "bob is now known as robert" {
		t.Errorf("nick system message = %q", got)
	}

	users := a.nextOfType("users_list").Users

	found := false

	for _, u := range users {
		if u.Nick == "robert" {
			found = true
		}
	}

	if !found {
		t.Errorf("users list does not contain robert: %+v", users)
	}
}

func TestTypingIndicator(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "TYPE", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "TYPE", "bob", "")
	defer b.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{Type: "typing"})

	users := a.nextOfType("users_list").Users

	for _, u := range users {
		if u.Nick == "bob" && !u.Typing {
			t.Errorf("bob not marked as typing: %+v", users)
		}
	}
}

func TestRateLimit(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RATE", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "RATE", "bob", "")
	defer b.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	for i := 0; i < 20; i++ {
		b.send(shared.Message{Type: "message", Text: fmt.Sprintf("msg %d", i)})
	}

	delivered := 0

	for {
		select {
		case m := <-a.msgs:
			if m.Type == "message" {
				delivered++
			}
		case <-time.After(300 * time.Millisecond):
			if delivered == 0 {
				t.Fatal("no messages delivered at all")
			}

			if delivered > 10 {
				t.Fatalf("rate limit not enforced: %d messages delivered in a burst", delivered)
			}

			return
		}
	}
}

func TestColorValidation(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "COLR", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	// Invalid color must be ignored (no users_list broadcast, no crash)
	a.send(shared.Message{Type: "color", Color: "not-a-color"})
	a.send(shared.Message{Type: "color", Color: "#12345"})

	// Valid color is applied
	a.send(shared.Message{Type: "color", Color: "#ff00aa"})

	users := a.nextOfType("users_list").Users

	for _, u := range users {
		if u.Nick == "alice" && u.Color != "#ff00aa" {
			t.Errorf("color not updated: %+v", users)
		}
	}
}

func TestJoinRejectsInvalidRoomCode(t *testing.T) {
	srv := startTestServer(t)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.WriteJSON(shared.Message{Type: "join", Nick: "x", Room: "AB"})

	conn.SetReadDeadline(time.Now().Add(time.Second))

	var m shared.Message

	if err := conn.ReadJSON(&m); err == nil {
		t.Errorf("expected connection to close for invalid room, got %+v", m)
	}
}

func TestJoinRequiresJoinMessageFirst(t *testing.T) {
	srv := startTestServer(t)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.WriteJSON(shared.Message{Type: "message", Text: "hi"})

	conn.SetReadDeadline(time.Now().Add(time.Second))

	var m shared.Message

	if err := conn.ReadJSON(&m); err == nil {
		t.Errorf("expected connection to close for non-join first message, got %+v", m)
	}
}

// TestConcurrentTraffic hammers the server with joins, leaves, users-list
// requests and nick changes while clients churn rooms. Run under -race this
// catches unsynchronized access to the rooms map and client fields.
func TestConcurrentTraffic(t *testing.T) {
	srv := startTestServer(t)

	stable := joinRoom(t, srv, "STAB", "stable", "")
	defer stable.close()

	twin := joinRoom(t, srv, "STAB", "twin", "")
	defer twin.close()

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Churn: rapidly join and leave rooms so rooms are created and deleted
	// concurrently with everything else.
	for i := 0; i < 4; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
				}

				conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
				if err != nil {
					continue
				}

				conn.WriteJSON(shared.Message{Type: "join", Nick: "churn", Room: "FLAP"})
				conn.SetReadDeadline(time.Now().Add(50 * time.Millisecond))

				for j := 0; j < 3; j++ {
					var m shared.Message

					if err := conn.ReadJSON(&m); err != nil {
						break
					}
				}

				conn.Close()
			}
		}()
	}

	// Hammer the discover endpoint concurrently so it races with nick changes.
	wg.Add(1)

	go func() {
		defer wg.Done()

		client := &http.Client{Timeout: time.Second}

		for {
			select {
			case <-stop:
				return
			default:
			}

			resp, err := client.Get(srv.URL + "/discover")
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}
	}()

	// Stable client alternates users-list requests and nick changes.
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 2000; i++ {
			select {
			case <-stop:
				return
			default:
			}

			if i%4 == 0 {
				stable.send(shared.Message{Type: "nick", NewNick: fmt.Sprintf("nick%d", i%7)})
			} else {
				stable.send(shared.Message{Type: "users"})
			}

			time.Sleep(time.Millisecond)
		}
	}()

	// Twin sends messages so users_list broadcasts fire while nick changes.
	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := 0; i < 2000; i++ {
			select {
			case <-stop:
				return
			default:
			}

			twin.send(shared.Message{Type: "message", Text: "ping"})

			time.Sleep(time.Millisecond)
		}
	}()

	time.Sleep(3 * time.Second)
	close(stop)
	wg.Wait()
}

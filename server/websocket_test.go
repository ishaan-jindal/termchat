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
	"unicode/utf8"

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
	resetMediaTokens()
}

func startTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(newMux())

	t.Cleanup(func() {
		srv.Close()
		waitForQuiescence(t)
		resetState(t)
	})

	return srv
}

// waitForQuiescence blocks until every read/write pump has exited, so tests
// can mutate the package-level limits without racing leftover goroutines
// from earlier connections.
func waitForQuiescence(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for livePumps.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("server pumps did not drain")
		}

		time.Sleep(time.Millisecond)
	}
}

// overrideLimit sets a package-level limit for the duration of the test,
// waiting for quiescence on both set and restore.
func overrideLimit[T any](t *testing.T, dst *T, val T) {
	t.Helper()

	waitForQuiescence(t)

	old := *dst
	*dst = val

	t.Cleanup(func() {
		waitForQuiescence(t)
		*dst = old
	})
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
	case <-time.After(10 * time.Second):
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

func TestUsersListCarriesServerTime(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "CLKT", "alice", "")
	defer a.close()

	before := time.Now().Unix()
	msg := a.nextOfType("users_list")
	after := time.Now().Unix()

	if msg.ServerTime < before || msg.ServerTime > after {
		t.Errorf("server_time = %d, want within [%d, %d]", msg.ServerTime, before, after)
	}

	if len(msg.Users) != 1 {
		t.Fatalf("users = %d, want 1", len(msg.Users))
	}

	if u := msg.Users[0]; u.JoinedAt <= 0 || u.JoinedAt > msg.ServerTime {
		t.Errorf("joined_at = %d, want positive and <= server_time %d", u.JoinedAt, msg.ServerTime)
	}
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

func TestUsersRequestRepliesToRequesterOnly(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "USRS", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b := joinRoom(t, srv, "USRS", "bob", "")
	defer b.close()

	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")

	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{Type: "users"})

	users := b.nextOfType("users_list").Users

	if len(users) != 2 {
		t.Fatalf("users = %d, want 2", len(users))
	}

	host := ""

	for _, u := range users {
		if u.IsHost {
			host = u.Nick
		}
	}

	if host != "alice" {
		t.Errorf("host = %q, want alice", host)
	}

	// The request is not a broadcast: alice's next frame is bob's message.
	b.send(shared.Message{Type: "message", Text: "ping"})

	next, ok := a.next()
	if !ok {
		t.Fatal("connection closed")
	}

	if next.Type != "message" {
		t.Errorf("alice received %q, want message", next.Type)
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

	waitUsers(t, a, "alice", "bob")

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

	waitUsers(t, a, "alice", "bob")

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

	waitUsers(t, a, "alice", "bob")

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

	waitUsers(t, a, "alice", "bob")

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

func TestHistoryCap(t *testing.T) {
	srv := startTestServer(t)

	overrideLimit(t, &maxMessagesPerSecond, 100)

	a := joinRoom(t, srv, "HIST", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	for i := 0; i < 40; i++ {
		a.send(shared.Message{Type: "message", Text: fmt.Sprintf("m-%d", i)})
	}

	// The server processes messages asynchronously. Echoes can be dropped
	// when the client's outbound buffer fills (32 slots), but history is
	// authoritative - it grows under the room lock for every message. Wait
	// until the last message is recorded.
	deadline := time.After(5 * time.Second)

	for {
		room := roomState(t, "HIST")
		room.Mutex.Lock()
		n := len(room.History)
		last := ""
		if n > 0 {
			last = room.History[n-1].Text
		}
		room.Mutex.Unlock()

		if n == 30 && last == "m-39" {
			break
		}

		select {
		case <-deadline:
			t.Fatalf("history never recorded all messages (len=%d, last=%q)", n, last)
		case <-time.After(20 * time.Millisecond):
		}
	}

	room := roomState(t, "HIST")
	room.Mutex.Lock()
	hist := make([]Message, len(room.History))
	copy(hist, room.History)
	room.Mutex.Unlock()

	if len(hist) != 30 {
		t.Fatalf("history length = %d, want 30", len(hist))
	}

	if hist[0].Text != "m-10" || hist[len(hist)-1].Text != "m-39" {
		t.Errorf("history window wrong: first=%q last=%q", hist[0].Text, hist[len(hist)-1].Text)
	}

	// A late joiner receives exactly the capped window.
	b := joinRoom(t, srv, "HIST", "bob", "")
	defer b.close()

	replay := b.nextOfType("history")

	if len(replay.Messages) != 30 {
		t.Fatalf("late joiner history = %d messages, want 30", len(replay.Messages))
	}

	if replay.Messages[0].Text != "m-10" || replay.Messages[29].Text != "m-39" {
		t.Errorf("late joiner history window wrong: first=%q last=%q",
			replay.Messages[0].Text, replay.Messages[29].Text)
	}
}

func TestMessageLengthTruncation(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "LENX", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "LENX", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	b.send(shared.Message{Type: "message", Text: strings.Repeat("x", 600)})

	msg := a.nextOfType("message")

	if len(msg.Text) != 500 {
		t.Errorf("message length = %d, want 500", len(msg.Text))
	}

	if !utf8.ValidString(msg.Text) {
		t.Error("truncated message is not valid UTF-8")
	}

	// Multi-byte runes must be truncated by rune, never split.
	b.send(shared.Message{Type: "message", Text: strings.Repeat("\U0001F600", 600)})

	msg = a.nextOfType("message")

	if got := utf8.RuneCountInString(msg.Text); got != 500 {
		t.Errorf("emoji message rune count = %d, want 500", got)
	}

	if !utf8.ValidString(msg.Text) {
		t.Error("emoji-truncated message is not valid UTF-8")
	}

	// Nicknames beyond 32 runes are rejected, not truncated.
	b.send(shared.Message{Type: "nick", NewNick: strings.Repeat("n", 40)})

	if got := b.nextOfType("error").Text; got != "invalid_nick" {
		t.Errorf("overlong nick error = %q", got)
	}
}

func TestJoinRejectsInvalidNickname(t *testing.T) {
	srv := startTestServer(t)

	for _, nick := range []string{"bad nick", "\u00e9clair", strings.Repeat("n", 33)} {
		c := joinRoom(t, srv, "NICK", nick, "")
		defer c.close()

		if got := c.nextOfType("error").Text; got != "invalid_nick" {
			t.Errorf("reply for %q = %q, want invalid_nick", nick, got)
		}

		if _, ok := c.next(); ok {
			t.Errorf("connection still open after rejecting %q", nick)
		}
	}
}

func TestJoinEmptyNicknameBecomesAnonymous(t *testing.T) {
	srv := startTestServer(t)

	c := joinRoom(t, srv, "ANON", "", "")
	defer c.close()

	c.nextOfType("history")

	users := c.nextOfType("users_list")

	if len(users.Users) != 1 || users.Users[0].Nick != "anonymous" {
		t.Errorf("users = %+v, want single anonymous", users.Users)
	}
}

func TestNickChangeRejectsInvalidName(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "NIKX", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "NIKX", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	b.send(shared.Message{Type: "nick", NewNick: "still bob"})

	if got := b.nextOfType("error").Text; got != "invalid_nick" {
		t.Errorf("invalid nick error = %q", got)
	}

	// The rejected change must not have been applied.
	a.send(shared.Message{Type: "users"})

	users := a.nextOfType("users_list")

	for _, u := range users.Users {
		if u.Nick == "bob" {
			continue
		}

		if u.Nick == "still bob" {
			t.Errorf("rejected nick was applied: %+v", users.Users)
		}
	}

	b.send(shared.Message{Type: "nick", NewNick: "robert"})
	waitUsers(t, a, "alice", "robert")
}

func TestPasswordRemoval(t *testing.T) {
	srv := startTestServer(t)

	host := joinRoom(t, srv, "PWRM", "host", "")
	defer host.close()

	host.nextOfType("history")
	host.nextOfType("system")
	host.nextOfType("users_list")

	host.send(shared.Message{Type: "set_password", Password: "secret"})
	host.nextOfType("system")

	// Locked: joining without a password is rejected.
	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}

	conn.WriteJSON(shared.Message{Type: "join", Nick: "x", Room: "PWRM"})

	var reply shared.Message

	if err := conn.ReadJSON(&reply); err != nil {
		t.Fatalf("read error reply: %v", err)
	}

	if reply.Type != "error" || reply.Text != "invalid_password" {
		t.Errorf("reply = %+v, want invalid_password", reply)
	}
	conn.Close()

	// Host removes the password.
	host.send(shared.Message{Type: "set_password", Password: ""})
	host.nextOfType("system")

	// Unlocked: joining without a password succeeds.
	guest := joinRoom(t, srv, "PWRM", "guest", "")
	defer guest.close()

	guest.nextOfType("history")
	guest.nextOfType("system")
	guest.nextOfType("users_list")
}

func TestHostSuccessionChain(t *testing.T) {
	srv := startTestServer(t)

	// Joins are processed asynchronously, so join and drain sequentially.
	a := joinRoom(t, srv, "CHAI", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b := joinRoom(t, srv, "CHAI", "bob", "")
	defer b.close()

	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")

	c := joinRoom(t, srv, "CHAI", "carol", "")
	defer c.close()

	c.nextOfType("history")
	c.nextOfType("system")
	c.nextOfType("users_list")
	a.nextOfType("system")
	a.nextOfType("users_list")
	b.nextOfType("system")
	b.nextOfType("users_list")

	if host := hostOf(t, "CHAI"); host != "alice" {
		t.Fatalf("host = %q, want alice", host)
	}

	a.close()

	if got := c.nextOfType("system").Text; got != "alice left the room" {
		t.Errorf("leave message = %q", got)
	}

	if got := c.nextOfType("system").Text; got != "bob is now the host" {
		t.Errorf("succession message = %q", got)
	}

	if host := hostOf(t, "CHAI"); host != "bob" {
		t.Fatalf("host after alice left = %q, want bob", host)
	}

	b.close()

	if got := c.nextOfType("system").Text; got != "bob left the room" {
		t.Errorf("leave message = %q", got)
	}

	if got := c.nextOfType("system").Text; got != "carol is now the host" {
		t.Errorf("succession message = %q", got)
	}

	if host := hostOf(t, "CHAI"); host != "carol" {
		t.Errorf("host after bob left = %q, want carol", host)
	}
}

func TestIdleDisconnect(t *testing.T) {
	srv := startTestServer(t)

	overrideLimit(t, &idleTimeout, 150*time.Millisecond)

	c := joinRoom(t, srv, "IDLE", "idler", "")
	defer c.close()

	c.nextOfType("history")
	c.nextOfType("system")
	c.nextOfType("users_list")

	time.Sleep(300 * time.Millisecond)

	cleanupIdleClientsOnce()

	// The read loop must hit an error and close the channel.
	deadline := time.After(3 * time.Second)

	for {
		select {
		case _, ok := <-c.msgs:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("client not disconnected after idle timeout")
		}
	}
}

func TestTypingExpiry(t *testing.T) {
	srv := startTestServer(t)

	overrideLimit(t, &typingExpiry, 150*time.Millisecond)

	a := joinRoom(t, srv, "TYPE", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "TYPE", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	b.send(shared.Message{Type: "typing"})

	if !isTyping(a.nextOfType("users_list").Users, "bob") {
		t.Fatal("bob not marked as typing")
	}

	time.Sleep(300 * time.Millisecond)

	cleanupTypingIndicatorsOnce()

	if isTyping(a.nextOfType("users_list").Users, "bob") {
		t.Fatal("bob still marked as typing after expiry")
	}
}

func isTyping(users []UserInfo, nick string) bool {
	for _, u := range users {
		if u.Nick == nick {
			return u.Typing
		}
	}

	return false
}

func waitUsers(t *testing.T, c *testClient, nicks ...string) []UserInfo {
	t.Helper()

	deadline := time.After(10 * time.Second)

	for {
		select {
		case m := <-c.msgs:
			if m.Type != "users_list" {
				continue
			}

			have := map[string]bool{}

			for _, u := range m.Users {
				have[u.Nick] = true
			}

			all := true

			for _, n := range nicks {
				if !have[n] {
					all = false

					break
				}
			}

			if all {
				return m.Users
			}
		case <-deadline:
			t.Fatalf("timed out waiting for users list with %v", nicks)
		}
	}
}

func TestPingEmission(t *testing.T) {
	srv := startTestServer(t)

	overrideLimit(t, &pingPeriod, 150*time.Millisecond)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	conn.WriteJSON(shared.Message{Type: "join", Nick: "pinger", Room: "PING"})

	pings := 0
	conn.SetPingHandler(func(string) error {
		pings++

		return nil
	})

	conn.SetReadDeadline(time.Now().Add(700 * time.Millisecond))

	for {
		var m shared.Message

		if err := conn.ReadJSON(&m); err != nil {
			break
		}
	}

	if pings == 0 {
		t.Error("server did not emit any ping frames")
	}
}

func TestRateLimitWindow(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RATE", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "RATE", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	for i := 0; i < 6; i++ {
		b.send(shared.Message{Type: "message", Text: fmt.Sprintf("burst-%d", i)})
	}

	delivered := 0

	collect := time.After(400 * time.Millisecond)

	for {
		select {
		case m := <-a.msgs:
			if m.Type == "message" {
				delivered++
			}
		case <-collect:
			if delivered != 5 {
				t.Fatalf("burst delivered %d messages, want exactly 5", delivered)
			}

			goto afterWindow
		}
	}

afterWindow:
	// Once the one-second window passes, messages flow again.
	time.Sleep(1100 * time.Millisecond)

	b.send(shared.Message{Type: "message", Text: "after-window"})

	if got := a.nextOfType("message").Text; got != "after-window" {
		t.Fatalf("message after window = %q, want after-window", got)
	}
}

func TestWhitespaceOnlyMessagesDropped(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "BLNK", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "BLNK", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	b.send(shared.Message{Type: "message", Text: "   "})
	b.send(shared.Message{Type: "message", Text: "\x1b[31m\x1b[0m"})

	select {
	case m := <-a.msgs:
		if m.Type == "message" {
			t.Fatalf("whitespace-only message was broadcast: %+v", m)
		}
	case <-time.After(400 * time.Millisecond):
	}
}

func TestMessageIDOrdering(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "IDRO", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	b := joinRoom(t, srv, "IDRO", "bob", "")
	defer b.close()

	b.nextOfType("history")
	b.nextOfType("system")
	b.nextOfType("users_list")

	a.nextOfType("system")
	a.nextOfType("users_list")

	b.send(shared.Message{Type: "message", Text: "one"})
	b.send(shared.Message{Type: "message", Text: "two"})
	b.send(shared.Message{Type: "message", Text: "three"})

	want := int64(3)

	for _, text := range []string{"one", "two", "three"} {
		m := a.nextOfType("message")

		if m.Text != text || m.ID != want {
			t.Errorf("message = %+v, want text %q id %d", m, text, want)
		}

		want++
	}

	// A late joiner replays history with the same IDs: 2 join system
	// messages then the 3 chat messages.
	c := joinRoom(t, srv, "IDRO", "carol", "")
	defer c.close()

	hist := c.nextOfType("history")

	if len(hist.Messages) != 5 {
		t.Fatalf("history = %d messages, want 5", len(hist.Messages))
	}

	for i, m := range hist.Messages {
		if m.ID != int64(i+1) {
			t.Errorf("history[%d].ID = %d, want %d", i, m.ID, i+1)
		}
	}
}

func TestReplyResolution(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "REPL", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "REPL", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	b.send(shared.Message{Type: "message", Text: "original"})

	b.nextOfType("message")
	target := a.nextOfType("message")

	if target.ID == 0 {
		t.Fatal("message has no ID")
	}

	// Alice replies to bob's message; the server resolves the quote.
	a.send(shared.Message{Type: "message", Text: "replying", ReplyToID: target.ID})

	reply := b.nextOfType("message")

	if reply.ReplyToID != target.ID {
		t.Errorf("reply.ReplyToID = %d, want %d", reply.ReplyToID, target.ID)
	}

	if reply.ReplyToNick != "bob" || reply.ReplyToText != "original" {
		t.Errorf("resolved quote = %q/%q, want bob/original", reply.ReplyToNick, reply.ReplyToText)
	}

	// Unknown targets deliver the message plain.
	a.send(shared.Message{Type: "message", Text: "ghost", ReplyToID: 99999})

	plain := b.nextOfType("message")

	if plain.ReplyToID != 0 || plain.ReplyToNick != "" || plain.ReplyToText != "" {
		t.Errorf("unknown-target reply = %+v, want plain message", plain)
	}

	// System messages are not quoteable.
	a.send(shared.Message{Type: "message", Text: "sys", ReplyToID: 2})

	plain = b.nextOfType("message")

	if plain.ReplyToID != 0 {
		t.Errorf("system-target reply = %+v, want plain message", plain)
	}
}

func TestReactionToggleAndCounts(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RACT", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "RACT", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	a.send(shared.Message{Type: "message", Text: "ping"})

	target := b.nextOfType("message")

	if target.ID == 0 {
		t.Fatal("message has no ID")
	}

	// Bob reacts +1; both clients see the count.
	b.send(shared.Message{Type: "reaction", ID: target.ID, Text: "+1"})

	a.nextOfType("reaction")
	b.nextOfType("reaction")

	// Alice reacts too: count 2.
	a.send(shared.Message{Type: "reaction", ID: target.ID, Text: "+1"})

	a.nextOfType("reaction")
	update := b.nextOfType("reaction")

	if len(update.Reactions) != 1 || update.Reactions[0].Count != 2 {
		t.Errorf("reaction update = %+v, want +1 x2", update)
	}

	// Bob toggles off: count 1.
	b.send(shared.Message{Type: "reaction", ID: target.ID, Text: "+1"})

	b.nextOfType("reaction")
	update = a.nextOfType("reaction")

	if len(update.Reactions) != 1 || update.Reactions[0].Count != 1 {
		t.Errorf("reaction update = %+v, want +1 x1", update)
	}

	// Last voter toggles off: the message has no reactions left.
	a.send(shared.Message{Type: "reaction", ID: target.ID, Text: "+1"})

	a.nextOfType("reaction")
	update = b.nextOfType("reaction")

	if len(update.Reactions) != 0 {
		t.Errorf("reaction update = %+v, want empty", update)
	}
}

func TestReactionBroadcastCarriesReactorNick(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RACN", "alice", "")
	defer a.close()

	b := joinRoom(t, srv, "RACN", "bob", "")
	defer b.close()

	waitUsers(t, a, "alice", "bob")

	a.send(shared.Message{Type: "message", Text: "ping"})

	target := b.nextOfType("message")

	b.send(shared.Message{Type: "reaction", ID: target.ID, Text: "+1"})

	update := a.nextOfType("reaction")

	if update.Nick != "bob" {
		t.Errorf("reaction frame nick = %q, want bob", update.Nick)
	}

	if len(update.Reactions) != 1 || update.Reactions[0].Name != "+1" {
		t.Errorf("reaction frame counts = %+v, want +1 x1", update.Reactions)
	}
}

func TestReactionInvalidTargetsIgnored(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RACI", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	// Unknown message ID, unknown reaction name, and system message target.
	a.send(shared.Message{Type: "reaction", ID: 999, Text: "+1"})
	a.send(shared.Message{Type: "reaction", ID: 1, Text: "bogus"})
	a.send(shared.Message{Type: "reaction", ID: 1, Text: "+1"})

	select {
	case m := <-a.msgs:
		if m.Type == "reaction" {
			t.Fatalf("unexpected reaction broadcast: %+v", m)
		}
	case <-time.After(400 * time.Millisecond):
	}
}

func TestReactionReplayInHistory(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "RARP", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	a.send(shared.Message{Type: "message", Text: "ping"})

	target := a.nextOfType("message")

	a.send(shared.Message{Type: "reaction", ID: target.ID, Text: "heart"})
	a.nextOfType("reaction")

	// A late joiner replays history with reactions embedded.
	b := joinRoom(t, srv, "RARP", "bob", "")
	defer b.close()

	hist := b.nextOfType("history")

	if len(hist.Messages) != 2 {
		t.Fatalf("history = %d messages, want 2", len(hist.Messages))
	}

	var ping *shared.Message

	for i := range hist.Messages {
		if hist.Messages[i].Text == "ping" {
			ping = &hist.Messages[i]
		}
	}

	if ping == nil {
		t.Fatal("ping message missing from history")
	}

	if len(ping.Reactions) != 1 || ping.Reactions[0].Name != "heart" || ping.Reactions[0].Count != 1 {
		t.Errorf("replayed reactions = %+v, want heart x1", ping.Reactions)
	}
}

func TestMediaTokenIssuance(t *testing.T) {
	srv := startTestServer(t)

	a := joinRoom(t, srv, "TOKN", "alice", "")
	defer a.close()

	a.send(shared.Message{Type: "media_token"})
	first := a.nextOfType("media_token").Token

	if first == "" {
		t.Fatal("empty media token")
	}

	a.send(shared.Message{Type: "media_token"})
	second := a.nextOfType("media_token").Token

	if second == "" || second == first {
		t.Fatalf("tokens not unique: %q vs %q", first, second)
	}

	room := roomState(t, "TOKN")
	room.Mutex.Lock()
	host := room.Host
	room.Mutex.Unlock()

	if got := consumeMediaToken(first); got != host {
		t.Fatal("token did not redeem to its client")
	}

	if consumeMediaToken(first) != nil {
		t.Fatal("token was redeemed twice")
	}
}

func TestMediaTokenExpiry(t *testing.T) {
	overrideLimit(t, &mediaTokenTTL, 20*time.Millisecond)

	srv := startTestServer(t)

	a := joinRoom(t, srv, "TOKE", "alice", "")
	defer a.close()

	a.send(shared.Message{Type: "media_token"})
	token := a.nextOfType("media_token").Token

	time.Sleep(50 * time.Millisecond)

	if consumeMediaToken(token) != nil {
		t.Fatal("expired token accepted")
	}
}

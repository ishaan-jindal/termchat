package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"termchat/shared"

	"github.com/gorilla/websocket"
)

var serverBin string

// TestMain builds the real server binary once so the process-level E2E test
// can exec it.
func TestMain(m *testing.M) {
	root := findRepoRoot()

	if root != "" {
		tmp, err := os.MkdirTemp("", "termchat-e2e-*")
		if err == nil {
			serverBin = filepath.Join(tmp, "termchat-server")

			cmd := exec.Command("go", "build", "-o", serverBin, "./server/cmd/server")
			cmd.Dir = root

			if out, err := cmd.CombinedOutput(); err != nil {
				fmt.Fprintf(os.Stderr, "building server binary: %v\n%s", err, out)
				serverBin = ""
			}
		}
	}

	code := m.Run()

	if serverBin != "" {
		os.RemoveAll(filepath.Dir(serverBin))
	}

	os.Exit(code)
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)

		if parent == dir {
			return ""
		}

		dir = parent
	}
}

// e2eClient drives the real CLI networking layer against a real server.
type e2eClient struct {
	t    *testing.T
	conn *Connection
	once sync.Once
}

func e2eConnect(t *testing.T, url, room, nick, password string) *e2eClient {
	t.Helper()

	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}

	go writePump(conn)

	conn, err = joinRoom(conn, url, room, nick, password, strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	return &e2eClient{t: t, conn: conn}
}

func (c *e2eClient) send(m Message) {
	c.t.Helper()

	select {
	case c.conn.Send <- m:
	case <-time.After(10 * time.Second):
		c.t.Fatal("timed out sending message")
	}
}

func (c *e2eClient) next() (Message, bool) {
	c.t.Helper()

	if c.conn.firstMsg != nil {
		m := *c.conn.firstMsg
		c.conn.firstMsg = nil

		return m, true
	}

	c.conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	var m Message

	if err := c.conn.conn.ReadJSON(&m); err != nil {
		return Message{}, false
	}

	return m, true
}

func (c *e2eClient) nextOfType(typ string) Message {
	c.t.Helper()

	for {
		m, ok := c.next()
		if !ok {
			c.t.Fatalf("connection closed waiting for %q", typ)
		}

		if m.Type == typ {
			return m
		}
	}
}

func (c *e2eClient) close() {
	c.once.Do(func() {
		close(c.conn.done)
		c.conn.conn.Close()
	})
}

// waitForRoomRemoval polls /discover until the room is gone.
func waitForRoomRemoval(addr, room string) error {
	deadline := time.Now().Add(5 * time.Second)

	for {
		resp, err := http.Get("http://" + addr + "/discover")
		if err != nil {
			return err
		}

		var rooms []shared.RoomInfo

		decodeErr := json.NewDecoder(resp.Body).Decode(&rooms)
		resp.Body.Close()

		if decodeErr == nil {
			for _, r := range rooms {
				if r.ID == room {
					if time.Now().After(deadline) {
						return fmt.Errorf("room %s still listed in discover", room)
					}

					time.Sleep(50 * time.Millisecond)

					continue
				}
			}

			return nil
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("room %s still listed in discover", room)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func TestE2EFullLifecycle(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	alice := e2eConnect(t, url, "E2E1", "alice", "")
	defer alice.close()

	alice.nextOfType("history")
	alice.nextOfType("system")

	bob := e2eConnect(t, url, "E2E1", "bob", "")
	defer bob.close()

	bob.nextOfType("history")
	bob.nextOfType("system")

	// Alice and bob both see the join broadcast.
	alice.nextOfType("system")
	alice.nextOfType("users_list")

	// Bob sends a message; alice receives it with attribution.
	bob.send(Message{Type: "message", Text: "hello from bob"})

	msg := alice.nextOfType("message")

	if msg.Nick != "bob" || msg.Text != "hello from bob" {
		t.Errorf("message = %+v, want bob/hello from bob", msg)
	}

	// Nick change propagates through the users list.
	bob.send(Message{Type: "nick", NewNick: "robert"})

	sys := alice.nextOfType("system")

	if !strings.Contains(sys.Text, "bob is now known as robert") {
		t.Errorf("nick system message = %q", sys.Text)
	}

	users := alice.nextOfType("users_list").Users

	seenBob := false

	for _, u := range users {
		if u.Nick == "robert" {
			seenBob = true
		}
	}

	if !seenBob {
		t.Errorf("users list missing robert: %+v", users)
	}

	// The room shows up in online discovery.
	resp, err := http.Get("http://" + addr + "/discover")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var rooms []shared.RoomInfo

	if err := json.NewDecoder(resp.Body).Decode(&rooms); err != nil {
		t.Fatal(err)
	}

	found := false

	for _, r := range rooms {
		if r.ID == "E2E1" {
			found = true

			if r.UserCount != 2 {
				t.Errorf("room user count = %d, want 2", r.UserCount)
			}
		}
	}

	if !found {
		t.Errorf("room E2E1 not listed in discover: %+v", rooms)
	}

	// Disconnect; the room is deleted when empty.
	bob.close()
	alice.close()

	if err := waitForRoomRemoval(addr, "E2E1"); err != nil {
		t.Error(err)
	}
}

func TestE2EPasswordLifecycle(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	host := e2eConnect(t, url, "E2E2", "host", "")
	defer host.close()

	host.nextOfType("history")
	host.nextOfType("system")

	host.send(Message{Type: "set_password", Password: "hunter2"})
	host.nextOfType("system")

	// A join without the password is rejected with an error frame.
	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}

	go writePump(conn)

	conn.Send <- Message{Type: "join", Nick: "eve", Room: "E2E2"}

	var reply Message

	if err := conn.conn.ReadJSON(&reply); err != nil {
		t.Fatal(err)
	}

	if reply.Type != "error" || reply.Text != "invalid_password" {
		t.Errorf("reply = %+v, want invalid_password", reply)
	}

	close(conn.done)
	conn.conn.Close()

	// With the password, the join succeeds.
	guest := e2eConnect(t, url, "E2E2", "guest", "hunter2")
	defer guest.close()

	guest.nextOfType("history")
	guest.nextOfType("system")

	// Host unlocks; a passwordless join now works.
	host.send(Message{Type: "set_password", Password: ""})
	host.nextOfType("system")

	open := e2eConnect(t, url, "E2E2", "stranger", "")
	defer open.close()

	open.nextOfType("history")
	open.nextOfType("system")
}

func TestE2EHistoryReplay(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	alice := e2eConnect(t, url, "E2E3", "alice", "")
	defer alice.close()

	alice.nextOfType("history")
	alice.nextOfType("system")

	for i := 0; i < 5; i++ {
		alice.send(Message{Type: "message", Text: fmt.Sprintf("msg-%d", i)})
	}

	// Wait for all five echoes before the late joiner arrives.
	for i := 0; i < 5; i++ {
		if m := alice.nextOfType("message"); m.Text != fmt.Sprintf("msg-%d", i) {
			t.Errorf("echo %d = %q", i, m.Text)
		}
	}

	bob := e2eConnect(t, url, "E2E3", "bob", "")
	defer bob.close()

	history := bob.nextOfType("history")

	// History holds system messages too, so 1 join notice + 5 messages.
	if len(history.Messages) != 6 {
		t.Fatalf("replayed history = %d messages, want 6", len(history.Messages))
	}

	for i, m := range history.Messages[1:] {
		if m.Text != fmt.Sprintf("msg-%d", i) {
			t.Errorf("replayed message %d = %q, want msg-%d", i, m.Text, i)
		}
	}
}

func TestE2EHostSuccession(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	alice := e2eConnect(t, url, "E2E4", "alice", "")
	bob := e2eConnect(t, url, "E2E4", "bob", "")

	alice.nextOfType("history")
	alice.nextOfType("system")
	bob.nextOfType("history")
	bob.nextOfType("system")
	alice.nextOfType("system")
	alice.nextOfType("users_list")

	alice.close()

	if got := bob.nextOfType("system").Text; got != "alice left the room" {
		t.Errorf("leave message = %q", got)
	}

	if got := bob.nextOfType("system").Text; got != "bob is now the host" {
		t.Errorf("succession message = %q", got)
	}

	// Discovery reflects the new host.
	deadline := time.Now().Add(5 * time.Second)

	for {
		resp, err := http.Get("http://" + addr + "/discover")
		if err != nil {
			t.Fatal(err)
		}

		var rooms []shared.RoomInfo

		decodeErr := json.NewDecoder(resp.Body).Decode(&rooms)
		resp.Body.Close()

		if decodeErr == nil {
			for _, r := range rooms {
				if r.ID == "E2E4" && r.HostNick == "bob" {
					bob.close()

					return
				}
			}
		}

		if time.Now().After(deadline) {
			t.Fatal("discovery never reported bob as host")
		}

		time.Sleep(50 * time.Millisecond)
	}
}

// TestE2EServerBinaryGracefulShutdown execs the real server binary, connects
// a real client, sends SIGTERM, and verifies the shutdown broadcast.
func TestE2EServerBinaryGracefulShutdown(t *testing.T) {
	if serverBin == "" {
		t.Skip("server binary not built")
	}

	addr := freeAddr(t)

	cmd := exec.Command(serverBin)
	cmd.Env = append(os.Environ(),
		"WS_HOST=127.0.0.1",
		"WS_PORT="+strings.TrimPrefix(addr, "127.0.0.1:"),
		"GITHUB_REPO=ishaan-jindal/termchat",
	)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	exited := make(chan error, 1)

	go func() {
		exited <- cmd.Wait()
	}()

	defer func() {
		select {
		case <-exited:
		default:
			cmd.Process.Kill()
		}
	}()

	// Wait for the HTTP listener.
	deadline := time.Now().Add(10 * time.Second)

	for {
		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				break
			}
		}

		if time.Now().After(deadline) {
			t.Fatal("server binary did not become healthy")
		}

		time.Sleep(50 * time.Millisecond)
	}

	// Bootstrap route works in the real binary.
	resp, err := http.Get("http://" + addr + "/FROG")
	if err != nil {
		t.Fatal(err)
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(body), `ROOM="FROG"`) {
		t.Errorf("bootstrap route did not render: %s", body)
	}

	// Connect a real client.
	conn, err := connectWebSocket("ws://" + addr + "/ws")
	if err != nil {
		t.Fatal(err)
	}

	go writePump(conn)

	conn.Send <- Message{Type: "join", Nick: "shutdown-test", Room: "SDWN"}

	var first Message

	if err := conn.conn.ReadJSON(&first); err != nil {
		t.Fatal(err)
	}

	// Send SIGTERM and expect the shutdown broadcast, then a closed conn.
	cmd.Process.Signal(syscall.SIGTERM)

	gotShutdown := false

	conn.conn.SetReadDeadline(time.Now().Add(5 * time.Second))

	for {
		var m Message

		if err := conn.conn.ReadJSON(&m); err != nil {
			break
		}

		if m.Type == "system" && strings.Contains(m.Text, "server shutting down") {
			gotShutdown = true
		}
	}

	if !gotShutdown {
		t.Error("client did not receive the shutdown broadcast")
	}

	// The process must exit on its own.
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("server process did not exit after SIGTERM")
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := l.Addr().String()
	l.Close()

	return addr
}

func TestE2EVoiceRelay(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	alice := e2eConnect(t, url, "VOIC", "alice", "")
	defer alice.close()

	bob := e2eConnect(t, url, "VOIC", "bob", "")
	defer bob.close()

	alice.nextOfType("history")
	alice.nextOfType("system")
	bob.nextOfType("history")
	bob.nextOfType("system")

	alice.send(Message{Type: "media_token"})
	atok := alice.nextOfType("media_token").Token

	bob.send(Message{Type: "media_token"})
	btok := bob.nextOfType("media_token").Token

	if atok == "" || btok == "" {
		t.Fatalf("empty media tokens: %q %q", atok, btok)
	}

	amc, err := dialMedia("ws://"+addr, "VOIC", atok)
	if err != nil {
		t.Fatal(err)
	}
	defer amc.close()

	bmc, err := dialMedia("ws://"+addr, "VOIC", btok)
	if err != nil {
		t.Fatal(err)
	}
	defer bmc.close()

	frame := shared.EncodeAudioFrame(
		shared.MediaKindAudio,
		shared.MediaCodecPCM16,
		0x1234,
		make([]byte, shared.AudioChunkBytes),
	)

	amc.conn.SetWriteDeadline(time.Now().Add(3 * time.Second))

	err = amc.conn.WriteMessage(websocket.BinaryMessage, frame)
	if err != nil {
		t.Fatalf("send voice frame: %v", err)
	}

	select {
	case got := <-bmc.inbox:
		_, _, voiceID, payload, ok := shared.ParseMediaFrame(got)

		if !ok {
			t.Fatal("relayed frame does not parse")
		}

		if voiceID == 0 || voiceID == 0x1234 {
			t.Errorf("voiceID = %#x, want a server-assigned stamp", voiceID)
		}

		if len(payload) != shared.AudioChunkBytes {
			t.Errorf("payload = %d bytes, want %d", len(payload), shared.AudioChunkBytes)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("peer never received the voice frame")
	}
}

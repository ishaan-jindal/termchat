package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"termchat/shared"

	chatserver "termchat/server"

	"github.com/gorilla/websocket"
)

// startRealServer boots the actual server package on a free port and stops it
// when the test finishes.
func startRealServer(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := l.Addr().String()
	l.Close()

	chatserver.SetLogOutput(io.Discard)

	errCh := make(chan error, 1)

	go func() {
		errCh <- chatserver.StartServer(addr)
	}()

	t.Cleanup(chatserver.Stop)

	waitForTCP(t, addr)

	return addr
}

func waitForTCP(t *testing.T, addr string) {
	t.Helper()

	// Wait for the listener to come up.
	deadline := time.Now().Add(5 * time.Second)

	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()

			return
		}

		if time.Now().After(deadline) {
			t.Fatal("server did not start in time")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// echoWSServer is a minimal websocket server that echoes messages back.
func echoWSServer(t *testing.T) *httptest.Server {
	t.Helper()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			var msg Message

			if err := conn.ReadJSON(&msg); err != nil {
				return
			}

			conn.WriteJSON(msg)
		}
	}))

	t.Cleanup(srv.Close)

	return srv
}

func TestConnectWebSocket(t *testing.T) {
	srv := echoWSServer(t)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.conn.Close()

	if conn.Send == nil || conn.done == nil {
		t.Error("connection missing channels")
	}
}

func TestConnectWebSocketRefused(t *testing.T) {
	conn, err := connectWebSocket("ws://127.0.0.1:1/ws")
	if err == nil {
		conn.conn.Close()
		t.Fatal("expected connection error")
	}
}

func TestConnectWebSocketErrorMentionsURL(t *testing.T) {
	_, err := connectWebSocket("ws://127.0.0.1:1/ws")
	if err == nil {
		t.Fatal("expected connection error")
	}

	if !strings.Contains(err.Error(), "ws://127.0.0.1:1/ws") {
		t.Errorf("err = %q, want the server URL", err)
	}
}

func TestReconnectRecoversAfterRestart(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}

	go writePump(conn)

	conn, err = joinRoom(conn, url, "RCVR", "alice", "", strings.NewReader(""), io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	close(conn.done)
	conn.conn.Close()

	chatserver.Stop()

	// A fresh server on the same address stands in for the recovered
	// network; the client must rejoin it without prompting.
	chatserver.SetLogOutput(io.Discard)

	go func() {
		_ = chatserver.StartServer(addr)
	}()

	t.Cleanup(chatserver.Stop)
	waitForTCP(t, addr)

	cmd := reconnectCmd(url, "RCVR", "alice", "", "")
	msg, ok := cmd().(reconnectedMsg)
	if !ok {
		t.Fatalf("reconnect returned %T, want reconnectedMsg", cmd())
	}

	if msg.conn.firstMsg == nil || msg.conn.firstMsg.Type != "history" {
		t.Fatalf("first message = %+v, want history", msg.conn.firstMsg)
	}

	close(msg.conn.done)
	msg.conn.conn.Close()
}

func TestWritePump(t *testing.T) {
	srv := echoWSServer(t)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.conn.Close()

	go writePump(conn)

	conn.Send <- Message{Type: "message", Text: "hello"}

	conn.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	var echo Message

	if err := conn.conn.ReadJSON(&echo); err != nil {
		t.Fatalf("echo read: %v", err)
	}

	if echo.Text != "hello" {
		t.Errorf("echo text = %q, want hello", echo.Text)
	}

	// Closing done stops the pump without panicking.
	close(conn.done)
	time.Sleep(50 * time.Millisecond)
}

func TestWaitForMessage(t *testing.T) {
	srv := echoWSServer(t)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	conn, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.conn.Close()

	go writePump(conn)

	// Buffered first message is returned and cleared.
	first := Message{Type: "history", Text: "replay"}
	conn.firstMsg = &first

	cmd := waitForMessage(conn)

	msg, ok := cmd().(IncomingMessage)
	if !ok {
		t.Fatalf("cmd() returned %T, want IncomingMessage", cmd())
	}

	if msg.Text != "replay" {
		t.Errorf("first message text = %q, want replay", msg.Text)
	}

	if conn.firstMsg != nil {
		t.Error("firstMsg not cleared after consumption")
	}

	// Next call reads from the socket.
	conn.Send <- Message{Type: "message", Text: "from-wire"}

	cmd = waitForMessage(conn)

	msg, ok = cmd().(IncomingMessage)
	if !ok {
		t.Fatalf("second cmd() returned %T", msg)
	}

	if msg.Text != "from-wire" {
		t.Errorf("wire message text = %q, want from-wire", msg.Text)
	}
}

func TestJoinRoomFlow(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	t.Run("success", func(t *testing.T) {
		conn, err := connectWebSocket(url)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.conn.Close()

		go writePump(conn)

		conn, err = joinRoom(conn, url, "FLOW", "alice", "", strings.NewReader(""), io.Discard)
		if err != nil {
			t.Fatal(err)
		}

		if conn.firstMsg == nil || conn.firstMsg.Type != "history" {
			t.Fatalf("first message = %+v, want history", conn.firstMsg)
		}
	})

	t.Run("prompted for password", func(t *testing.T) {
		// Lock the room first. The host must stay connected until the guest
		// joined: an empty locked room is deleted and the password lost.
		host, err := connectWebSocket(url)
		if err != nil {
			t.Fatal(err)
		}

		go writePump(host)

		host, err = joinRoom(host, url, "FLOW", "host", "", strings.NewReader(""), io.Discard)
		if err != nil {
			t.Fatal(err)
		}

		host.Send <- Message{Type: "set_password", Password: "secret"}

		// Wait until discovery confirms the room is locked.
		if err := waitForLocked(addr, "FLOW"); err != nil {
			t.Fatal(err)
		}

		// Joining without a password prompts (we feed "secret") and succeeds.
		conn, err := connectWebSocket(url)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.conn.Close()

		go writePump(conn)

		prompt := ""

		conn, err = joinRoom(conn, url, "FLOW", "bob", "", strings.NewReader("secret\n"),
			&writerFunc{fn: func(p []byte) (int, error) { prompt += string(p); return len(p), nil }})
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(prompt, "Room requires a password") {
			t.Errorf("prompt = %q, want password prompt", prompt)
		}

		if conn.firstMsg == nil || conn.firstMsg.Type != "history" {
			t.Fatalf("first message after retry = %+v, want history", conn.firstMsg)
		}

		// Host can leave now; bob keeps the room alive.
		close(host.done)
		host.conn.Close()
	})

	t.Run("wrong password twice", func(t *testing.T) {
		conn, err := connectWebSocket(url)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.conn.Close()

		go writePump(conn)

		_, err = joinRoom(conn, url, "FLOW", "eve", "", strings.NewReader("wrong\n"), io.Discard)
		if err == nil || !strings.Contains(err.Error(), "wrong password") {
			t.Fatalf("err = %v, want wrong password", err)
		}
	})

	t.Run("invalid room rejected", func(t *testing.T) {
		conn, err := connectWebSocket(url)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.conn.Close()

		go writePump(conn)

		_, err = joinRoom(conn, url, "BAD", "x", "", strings.NewReader(""), io.Discard)
		if err == nil {
			t.Fatal("expected error for invalid room")
		}
	})
}

type writerFunc struct {
	fn func(p []byte) (int, error)
}

func (w *writerFunc) Write(p []byte) (int, error) {
	return w.fn(p)
}

func TestServerStop(t *testing.T) {
	addr := startRealServer(t)

	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d", resp.StatusCode)
	}

	chatserver.Stop()

	// After Stop, the listener must be gone.
	deadline := time.Now().Add(5 * time.Second)

	for {
		_, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err != nil {
			return
		}

		if time.Now().After(deadline) {
			t.Fatal("server still listening after Stop")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// waitForLocked polls /discover until the room reports HasPassword.
func waitForLocked(addr, room string) error {
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
					if r.HasPassword {
						return nil
					}

					break
				}
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("room %s never reported as locked", room)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

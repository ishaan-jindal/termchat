package server

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"termchat/shared"

	"github.com/gorilla/websocket"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("TERMCHAT_TEST_ENVOR", "set")

	if got := envOr("TERMCHAT_TEST_ENVOR", "fb"); got != "set" {
		t.Errorf("envOr set = %q, want set", got)
	}

	if got := envOr("TERMCHAT_TEST_ENVOR_MISSING", "fb"); got != "fb" {
		t.Errorf("envOr missing = %q, want fb", got)
	}

	t.Setenv("TERMCHAT_TEST_ENVOR", "")

	if got := envOr("TERMCHAT_TEST_ENVOR", "fb"); got != "fb" {
		t.Errorf("envOr empty = %q, want fb", got)
	}
}

func TestInitBootstrapConfig(t *testing.T) {
	t.Setenv("GITHUB_REPO", "acme/termchat")
	t.Setenv("PUBLIC_BASE_URL", "https://chat.example.com/")

	initBootstrapConfig()

	if githubRepo != "acme/termchat" {
		t.Errorf("githubRepo = %q, want acme/termchat", githubRepo)
	}

	if publicBaseURL != "https://chat.example.com" {
		t.Errorf("publicBaseURL = %q, want trimmed URL", publicBaseURL)
	}

	versionMu.RLock()
	v := cachedCLIVersion
	versionMu.RUnlock()

	if v != "cli-v0.0.0" {
		t.Errorf("cachedCLIVersion = %q, want cli-v0.0.0", v)
	}

	err := os.Unsetenv("GITHUB_REPO")
	if err != nil {
		t.Fatal(err)
	}

	err = os.Unsetenv("PUBLIC_BASE_URL")
	if err != nil {
		t.Fatal(err)
	}

	githubRepo = "ishaan-jindal/termchat"
	publicBaseURL = "http://localhost"

	initBootstrapConfig()

	if githubRepo != "ishaan-jindal/termchat" || publicBaseURL != "http://localhost" {
		t.Errorf("fallback failed: repo=%q base=%q", githubRepo, publicBaseURL)
	}

	t.Cleanup(func() {
		githubRepo = "ishaan-jindal/termchat"
		publicBaseURL = "http://localhost"

		versionMu.Lock()
		cachedCLIVersion = "cli-v0.0.0"
		versionMu.Unlock()
	})
}

func TestSetInitialPassword(t *testing.T) {
	old := initialPassword

	t.Cleanup(func() { initialPassword = old })

	SetInitialPassword("s3cret")

	if initialPassword != "s3cret" {
		t.Errorf("initialPassword = %q, want s3cret", initialPassword)
	}

	SetInitialPassword("")

	if initialPassword != "" {
		t.Errorf("initialPassword = %q, want empty", initialPassword)
	}
}

func TestSetLogOutputNil(t *testing.T) {
	SetLogOutput(nil)

	t.Cleanup(func() { SetLogOutput(io.Discard) })
}

func TestMediaConnTrySend(t *testing.T) {
	mc := &mediaConn{send: make(chan []byte, 1), done: make(chan struct{})}

	if !mc.trySend([]byte("a")) {
		t.Fatal("trySend on empty buffer = false, want true")
	}

	if got := <-mc.send; string(got) != "a" {
		t.Fatalf("sent = %q, want a", got)
	}

	mc.send <- []byte("jam")

	if mc.trySend([]byte("y")) {
		t.Error("trySend on full buffer = true, want false")
	}

	if got := <-mc.send; string(got) != "jam" {
		t.Fatalf("sent = %q, want jam", got)
	}

	done := make(chan struct{})
	close(done)

	closed := &mediaConn{send: make(chan []byte, 1), done: done}
	closed.send <- []byte("jam")

	if closed.trySend([]byte("z")) {
		t.Error("trySend on closed conn = true, want false")
	}
}

func TestMediaCloseAllAndResetHub(t *testing.T) {
	srv := startTestServer(t)

	t.Cleanup(resetMediaHub)

	a := joinRoom(t, srv, "CLOZ", "alice", "")
	defer a.close()

	a.nextOfType("history")
	a.nextOfType("system")
	a.nextOfType("users_list")

	mc := dialMedia(t, srv, requestMediaToken(t, a), "CLOZ")
	defer mc.Close()

	hub.mu.Lock()
	n := len(hub.conns)
	hub.mu.Unlock()

	if n != 1 {
		t.Fatalf("hub conns = %d, want 1", n)
	}

	hub.closeAll()

	err := mc.SetReadDeadline(time.Now().Add(5 * time.Second))
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := mc.NextReader(); err == nil {
		t.Error("media conn survived closeAll")
	}

	resetMediaHub()

	hub.mu.Lock()
	left := len(hub.conns) + len(hub.streams)
	hub.mu.Unlock()

	if left != 0 {
		t.Errorf("hub entries after reset = %d, want 0", left)
	}
}

func TestCleanupLoopsStop(t *testing.T) {
	loops := []struct {
		name string
		fn   func(<-chan struct{})
	}{
		{"idle", cleanupIdleClients},
		{"typing", cleanupTypingIndicators},
	}

	for _, loop := range loops {
		t.Run(loop.name, func(t *testing.T) {
			stop := make(chan struct{})
			done := make(chan struct{})

			go func() {
				defer close(done)
				loop.fn(stop)
			}()

			close(stop)

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("cleanup loop did not stop")
			}
		})
	}
}

func TestRefreshCLIVersionLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"tag_name":"cli-v7.7.7"}`))
	}))
	defer srv.Close()

	oldBase, oldRepo := githubAPIBase, githubRepo

	t.Cleanup(func() {
		githubAPIBase = oldBase
		githubRepo = oldRepo
	})

	versionMu.Lock()
	oldVer := cachedCLIVersion
	cachedCLIVersion = ""
	versionMu.Unlock()

	t.Cleanup(func() {
		versionMu.Lock()
		cachedCLIVersion = oldVer
		versionMu.Unlock()
	})

	githubAPIBase = srv.URL
	githubRepo = "acme/termchat"

	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)
		refreshCLIVersionLoop(stop)
	}()

	deadline := time.Now().Add(5 * time.Second)

	for {
		versionMu.RLock()
		v := cachedCLIVersion
		versionMu.RUnlock()

		if v == "cli-v7.7.7" {
			break
		}

		if time.Now().After(deadline) {
			close(stop)
			<-done
			t.Fatal("loop never refreshed the version")
		}
	}

	close(stop)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh loop did not stop")
	}
}

func TestStartServerAndStop(t *testing.T) {
	waitForQuiescence(t)

	oldPassword := initialPassword

	t.Cleanup(func() {
		Stop()
		waitForQuiescence(t)

		roomsMutex.Lock()
		rooms = map[string]*Room{}
		roomsMutex.Unlock()

		resetMediaTokens()
		resetMediaHub()
		initialPassword = oldPassword

		stopMu.Lock()
		stopCh = nil
		drainCh = nil
		stopMu.Unlock()
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := ln.Addr().String()
	ln.Close()

	SetInitialPassword("s3cret")

	srvErr := make(chan error, 1)

	go func() { srvErr <- StartServer(addr) }()

	deadline := time.Now().Add(10 * time.Second)

	for {
		var pollErr error
		ready := false

		resp, err := http.Get("http://" + addr + "/healthz")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			ready = resp.StatusCode == http.StatusOK
		} else {
			pollErr = err
		}

		if ready {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("server on %s never became ready: %v", addr, pollErr)
		}

		time.Sleep(20 * time.Millisecond)
	}

	dial := func() *websocket.Conn {
		t.Helper()

		conn, _, err := websocket.DefaultDialer.Dial("ws://"+addr+"/ws", nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}

		return conn
	}

	host := dial()
	defer host.Close()

	err = host.WriteJSON(shared.Message{Type: "join", Nick: "host", Room: "INIT", Password: "s3cret"})
	if err != nil {
		t.Fatalf("host join write: %v", err)
	}

	host.SetReadDeadline(time.Now().Add(5 * time.Second))

	var hist shared.Message
	err = host.ReadJSON(&hist)
	if err != nil {
		t.Fatalf("host join reply: %v", err)
	}

	if hist.Type != "history" {
		t.Fatalf("host first frame = %q, want history", hist.Type)
	}

	// The first room inherits the initial password: no password is rejected.
	guest := dial()
	defer guest.Close()

	err = guest.WriteJSON(shared.Message{Type: "join", Nick: "guest", Room: "INIT"})
	if err != nil {
		t.Fatalf("guest join write: %v", err)
	}

	guest.SetReadDeadline(time.Now().Add(5 * time.Second))

	var reply shared.Message
	err = guest.ReadJSON(&reply)
	if err != nil {
		t.Fatalf("guest reply: %v", err)
	}

	if reply.Type != "error" || reply.Text != "invalid_password" {
		t.Fatalf("guest reply = %+v, want invalid_password", reply)
	}

	Stop()

	select {
	case err := <-srvErr:
		if err != http.ErrServerClosed {
			t.Errorf("StartServer = %v, want ErrServerClosed", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("StartServer did not return after Stop")
	}

	host.SetReadDeadline(time.Now().Add(5 * time.Second))

	drained := time.Now().Add(5 * time.Second)

	for {
		var after shared.Message

		if err := host.ReadJSON(&after); err != nil {
			break
		}

		if time.Now().After(drained) {
			t.Error("client conn survived Stop")
			break
		}
	}
}

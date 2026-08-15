package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"termchat/shared"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	publicAPIURL = "https://chat.example.com"
	versionMu.Lock()
	cachedCLIVersion = "cli-v9.9.9"
	versionMu.Unlock()

	srv := httptest.NewServer(newRouter())
	t.Cleanup(srv.Close)

	return srv
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestNewRoomAPI(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/api/new")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	room := strings.TrimSpace(string(body))

	if !shared.IsValidRoomCode(room) {
		t.Fatalf("room = %q, want valid room code", room)
	}
}

func TestCreateRoomRendersBootstrap(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	if !strings.Contains(s, "ROOM=") {
		t.Error("bootstrap script missing ROOM variable")
	}

	if !strings.Contains(s, "https://chat.example.com") {
		t.Error("bootstrap script missing API URL")
	}

	if !strings.Contains(s, "cli-v9.9.9") {
		t.Error("bootstrap script missing CLI version")
	}
}

func TestCreateRoomGeneratesValidRoom(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "ROOM=") {
			room := strings.Trim(strings.TrimPrefix(line, "ROOM="), "\"")
			if !shared.IsValidRoomCode(room) {
				t.Fatalf("rendered room %q is not a valid room code", room)
			}
			return
		}
	}

	t.Fatal("no ROOM variable found in bootstrap script")
}

func TestJoinRoomRendersBootstrap(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/FROG")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `ROOM="FROG"`) {
		t.Errorf("bootstrap script does not contain ROOM=\"FROG\":\n%s", body)
	}
}

func TestJoinRoomRejectsInvalidCode(t *testing.T) {
	srv := newTestServer(t)

	for _, path := range []string{"/FRO", "/FROGS", "/FRO!", "/" + strings.Repeat("A", 32)} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400", path, resp.StatusCode)
		}
	}
}

func TestJoinRoomNormalizesCase(t *testing.T) {
	// Room codes are case-insensitive: /frog joins the same room as /FROG.
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/frog")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	if !strings.Contains(string(body), `ROOM="FROG"`) {
		t.Errorf("bootstrap script does not contain normalized ROOM=\"FROG\":\n%s", body)
	}
}

func TestWindowsBootstrap(t *testing.T) {
	srv := newTestServer(t)

	resp, err := http.Get(srv.URL + "/win/7WHB")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	s := string(body)

	if !strings.Contains(s, `[string]$Room = "7WHB"`) {
		t.Errorf("powershell bootstrap missing room param:\n%s", s)
	}

	if !strings.Contains(s, "termchat-windows-amd64.exe") {
		t.Errorf("powershell bootstrap missing windows binary mapping:\n%s", s)
	}
}

func TestBinaryRedirect(t *testing.T) {
	t.Setenv("GITHUB_REPO", "acme/termchat")

	srv := newTestServer(t)

	// Do not follow the redirect; we only care about the 307 response.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(srv.URL + "/bin/termchat-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", resp.StatusCode)
	}

	loc := resp.Header.Get("Location")

	want := "https://github.com/acme/termchat/releases/latest/download/termchat-linux-amd64"

	if loc != want {
		t.Fatalf("location = %q, want %q", loc, want)
	}
}

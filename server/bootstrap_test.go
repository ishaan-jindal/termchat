package server

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"termchat/shared"
)

func initBootstrapTestEnv(t *testing.T) {
	t.Helper()

	githubRepo = "acme/termchat"
	publicBaseURL = "https://chat.example.com"

	versionMu.Lock()
	cachedCLIVersion = "cli-v9.9.9"
	versionMu.Unlock()

	t.Cleanup(func() {
		githubRepo = "ishaan-jindal/termchat"
		publicBaseURL = "http://localhost"
	})
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	return resp, string(body)
}

func TestHealthz(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/healthz")

	if resp.StatusCode != http.StatusOK || body != "ok" {
		t.Fatalf("status = %d, body = %q, want 200 ok", resp.StatusCode, body)
	}
}

func TestCreateRoomRendersBootstrap(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Fatalf("content-type = %q, want text/plain", ct)
	}

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "ROOM=") {
			room := strings.Trim(strings.TrimPrefix(line, "ROOM="), "\"")

			if !shared.IsValidRoomCode(room) {
				t.Fatalf("rendered room %q is not a valid room code", room)
			}

			if !strings.Contains(body, "https://chat.example.com") {
				t.Error("bootstrap script missing BASE_URL")
			}

			if !strings.Contains(body, "cli-v9.9.9") {
				t.Error("bootstrap script missing CLI version")
			}

			if !strings.Contains(body, `--server "$WS_URL"`) {
				t.Error("bootstrap script does not pass --server to the binary")
			}

			return
		}
	}

	t.Fatal("no ROOM variable found in bootstrap script")
}

func TestJoinRoomRendersBootstrap(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/FROG")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !strings.Contains(body, `ROOM="FROG"`) {
		t.Errorf("bootstrap script does not contain ROOM=\"FROG\":\n%s", body)
	}
}

func TestJoinRoomNormalizesCase(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/frog")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !strings.Contains(body, `ROOM="FROG"`) {
		t.Errorf("bootstrap script does not contain normalized ROOM=\"FROG\":\n%s", body)
	}
}

func TestJoinRoomRejectsInvalidCode(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	for _, path := range []string{"/FRO", "/FROGS", "/FRO!", "/" + strings.Repeat("A", 32)} {
		resp, body := get(t, srv.URL+path)

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s status = %d, want 400 (body: %s)", path, resp.StatusCode, body)
		}
	}
}

func TestWindowsBootstrap(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/win/7WHB")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	if !strings.Contains(body, `[string]$Room = "7WHB"`) {
		t.Errorf("powershell bootstrap missing room param:\n%s", body)
	}

	if !strings.Contains(body, "termchat-windows-amd64.exe") {
		t.Errorf("powershell bootstrap missing windows binary mapping:\n%s", body)
	}

	if !strings.Contains(body, `"--server", "$wsUrl"`) {
		t.Errorf("powershell bootstrap does not pass --server:\n%s", body)
	}
}

func TestWindowsCreateRoom(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	resp, body := get(t, srv.URL+"/win")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	roomRe := regexp.MustCompile(`\[string\]\$Room = "([A-Z0-9]{4})"`)
	m := roomRe.FindStringSubmatch(body)

	if m == nil {
		t.Fatalf("powershell bootstrap missing generated room:\n%s", body)
	}

	if !shared.IsValidRoomCode(m[1]) {
		t.Fatalf("generated room %q is invalid", m[1])
	}
}

func TestBinaryRedirect(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Get(srv.URL + "/bin/termchat-linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", resp.StatusCode)
	}

	want := "https://github.com/acme/termchat/releases/latest/download/termchat-linux-amd64"

	if loc := resp.Header.Get("Location"); loc != want {
		t.Fatalf("location = %q, want %q", loc, want)
	}
}

func TestBinaryRedirectRejectsUnknownNames(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	for _, name := range []string{
		"foo", "..%2fetc%2fpasswd", "termchat-linux-riscv64", "termchat-android-arm64",
	} {
		resp, body := get(t, srv.URL+"/bin/"+name)

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("GET /bin/%s status = %d, want 400 (body: %s)", name, resp.StatusCode, body)
		}
	}
}

func TestBootstrapRoutesDoNotShadowWebSocket(t *testing.T) {
	srv := startTestServer(t)
	initBootstrapTestEnv(t)

	// /ws and /discover must still be served by their handlers even though
	// /{room} would match a single segment.
	resp, _ := get(t, srv.URL+"/ws")

	if resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/plain") {
		t.Error("/ws was routed to the bootstrap handler")
	}

	resp, body := get(t, srv.URL+"/discover")

	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("/discover status = %d, body = %q, want JSON", resp.StatusCode, body)
	}
}

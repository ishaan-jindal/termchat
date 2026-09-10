package server

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"termchat/shared"
)

//go:embed scripts/bootstrap.sh
var bootstrapScript string

//go:embed scripts/bootstrap.ps1
var windowsBootstrapScript string

var (
	githubRepo       = "ishaan-jindal/termchat"
	githubAPIBase    = "https://api.github.com"
	publicBaseURL    = "http://localhost"
	cachedCLIVersion string
	versionMu        sync.RWMutex
)

// allowedBinaries restricts /bin/{binary} to the release asset names this
// project actually publishes.
var allowedBinaries = map[string]bool{
	"termchat-linux-amd64":       true,
	"termchat-linux-arm64":       true,
	"termchat-linux-386":         true,
	"termchat-darwin-amd64":      true,
	"termchat-darwin-arm64":      true,
	"termchat-windows-amd64.exe": true,
	"termchat-windows-arm64.exe": true,
	"termchat-android-arm64":     true,
}

func initBootstrapConfig() {
	// Guard against concurrent writes if more than one server ever runs in
	// this process (the background refresh also writes these).
	versionMu.Lock()
	defer versionMu.Unlock()

	githubRepo = envOr("GITHUB_REPO", githubRepo)
	publicBaseURL = strings.TrimSuffix(envOr("PUBLIC_BASE_URL", publicBaseURL), "/")

	// Fallback until the background refresh populates the real version.
	cachedCLIVersion = "cli-v0.0.0"
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return fallback
}

func latestCLIVersion() string {
	versionMu.RLock()
	defer versionMu.RUnlock()

	if cachedCLIVersion == "" {
		return "cli-v0.0.0"
	}

	return cachedCLIVersion
}

func createRoomHandler(w http.ResponseWriter, _ *http.Request) {
	renderBootstrapScript(w, shared.GenerateRoomCode())
}

func joinRoomHandler(w http.ResponseWriter, r *http.Request) {
	room := shared.NormalizeRoomCode(r.PathValue("room"))
	if !shared.IsValidRoomCode(room) {
		http.Error(w, "invalid room code", http.StatusBadRequest)
		return
	}

	renderBootstrapScript(w, room)
}

func windowsCreateRoomHandler(w http.ResponseWriter, _ *http.Request) {
	renderWindowsBootstrap(w, shared.GenerateRoomCode())
}

func windowsJoinHandler(w http.ResponseWriter, r *http.Request) {
	room := shared.NormalizeRoomCode(r.PathValue("room"))
	if !shared.IsValidRoomCode(room) {
		http.Error(w, "invalid room code", http.StatusBadRequest)
		return
	}

	renderWindowsBootstrap(w, room)
}

func binaryHandler(w http.ResponseWriter, r *http.Request) {
	binary := r.PathValue("binary")

	if !allowedBinaries[binary] {
		http.Error(w, "invalid binary name", http.StatusBadRequest)
		return
	}

	url := fmt.Sprintf(
		"https://github.com/%s/releases/latest/download/%s",
		githubRepo,
		binary,
	)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func renderBootstrapScript(w http.ResponseWriter, room string) {
	renderScript(w, bootstrapScript, room)
}

func renderWindowsBootstrap(w http.ResponseWriter, room string) {
	renderScript(w, windowsBootstrapScript, room)
}

func renderScript(w http.ResponseWriter, script, room string) {
	tmpl, err := template.New("bootstrap").Parse(script)
	if err != nil {
		http.Error(w, "failed to parse bootstrap script", http.StatusInternalServerError)
		return
	}

	data := map[string]string{
		"Room":    room,
		"BaseURL": publicBaseURL,
		"Version": latestCLIVersion(),
	}

	var out bytes.Buffer

	if err := tmpl.Execute(&out, data); err != nil {
		http.Error(w, "failed to render bootstrap script", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(out.Bytes())
}

func fetchLatestCLIVersion() string {
	versionMu.RLock()
	repo := githubRepo
	base := githubAPIBase
	versionMu.RUnlock()

	client := &http.Client{Timeout: 5 * time.Second}

	url := fmt.Sprintf(
		"%s/repos/%s/releases/latest",
		base,
		repo,
	)

	resp, err := client.Get(url)
	if err != nil {
		logger.Println(err)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger.Printf("github releases lookup returned %s", resp.Status)
		return ""
	}

	type release struct {
		TagName string `json:"tag_name"`
	}

	var r release

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		logger.Println(err)
		return ""
	}

	return r.TagName
}

func refreshCLIVersionLoop(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)

	defer ticker.Stop()

	// Fetch immediately, then refresh periodically. Never blocks startup.
	for {
		refreshCLIVersionOnce()

		select {
		case <-stop:
			return
		case <-ticker.C:
		}
	}
}

func refreshCLIVersionOnce() {
	version := fetchLatestCLIVersion()

	if version != "" {
		versionMu.Lock()
		cachedCLIVersion = version
		versionMu.Unlock()
		logger.Println("updated latest cli version:", version)
	}
}

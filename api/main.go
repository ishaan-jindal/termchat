package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	publicAPIURL     string
	publicWSURL      string
	latestCLIVersion string
)

func main() {
	apiPort := os.Getenv("API_PORT")

	publicAPIURL = os.Getenv("PUBLIC_API_URL")
	publicWSURL = os.Getenv("PUBLIC_WS_URL")
	latestCLIVersion = fetchLatestCLIVersion()

	go refreshCLIVersionLoop()

	r := chi.NewRouter()

	// Windows bootstrap
	r.Get("/win", windowsCreateRoomHandler)
	r.Get("/win/{room}", windowsJoinHandler)

	// Linux/macOS bootstrap
	r.Get("/", createRoomHandler)
	r.Get("/{room}", joinRoomHandler)

	// Binary downloads
	r.Get("/bin/{binary}", binaryHandler)

	addr := ":" + apiPort

	log.Println("api server running on", addr)

	err := http.ListenAndServe(addr, r)
	if err != nil {
		log.Fatal(err)
	}
}

func createRoomHandler(w http.ResponseWriter, r *http.Request) {
	room := generateRoomCode()

	renderBootstrapScript(w, room)
}

func joinRoomHandler(w http.ResponseWriter, r *http.Request) {
	room := chi.URLParam(r, "room")

	renderBootstrapScript(w, room)
}

func windowsJoinHandler(w http.ResponseWriter, r *http.Request) {
	room := chi.URLParam(r, "room")

	renderWindowsBootstrap(w, room)
}

func windowsCreateRoomHandler(w http.ResponseWriter, r *http.Request) {
	room := generateRoomCode()

	renderWindowsBootstrap(w, room)
}

func renderBootstrapScript(w http.ResponseWriter, room string) {
	content, err := os.ReadFile("scripts/bootstrap.sh")
	if err != nil {
		http.Error(w, "failed to load bootstrap script", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New("bootstrap").Parse(string(content))
	if err != nil {
		http.Error(w, "failed to parse bootstrap script", http.StatusInternalServerError)
		return
	}

	data := map[string]string{
		"Room":    room,
		"ApiURL":  publicAPIURL,
		"WsURL":   publicWSURL,
		"Version": latestCLIVersion,
	}

	var out bytes.Buffer

	err = tmpl.Execute(&out, data)
	if err != nil {
		http.Error(w, "failed to render bootstrap script", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	w.Write(out.Bytes())
}

func renderWindowsBootstrap(w http.ResponseWriter, room string) {
	content, err := os.ReadFile("scripts/bootstrap.ps1")
	if err != nil {
		http.Error(w, "failed to load bootstrap script", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.New("bootstrap").Parse(string(content))
	if err != nil {
		http.Error(w, "failed to parse bootstrap script", http.StatusInternalServerError)
		return
	}

	data := map[string]string{
		"Room":    room,
		"ApiURL":  publicAPIURL,
		"WsURL":   publicWSURL,
		"Version": latestCLIVersion,
	}

	var out bytes.Buffer

	err = tmpl.Execute(&out, data)
	if err != nil {
		http.Error(w, "failed to render bootstrap script", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	w.Write(out.Bytes())
}

func binaryHandler(w http.ResponseWriter, r *http.Request) {
	binary := chi.URLParam(r, "binary")

	repo := os.Getenv("GITHUB_REPO")

	url := fmt.Sprintf(
		"https://github.com/%s/releases/latest/download/%s",
		repo,
		binary,
	)

	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func generateRoomCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

	length := 4

	bytes := make([]byte, length)

	_, err := rand.Read(bytes)
	if err != nil {
		return "FROG"
	}

	result := make([]byte, length)

	for i, b := range bytes {
		result[i] = chars[int(b)%len(chars)]
	}

	return string(result)
}

func fetchLatestCLIVersion() string {
	repo := os.Getenv("GITHUB_REPO")

	url := fmt.Sprintf(
		"https://api.github.com/repos/%s/releases/latest",
		repo,
	)

	resp, err := http.Get(url)
	if err != nil {
		log.Println(err)
		return "cli-v0.0.0"
	}
	defer resp.Body.Close()

	type release struct {
		TagName string `json:"tag_name"`
	}

	var r release

	err = json.NewDecoder(resp.Body).Decode(&r)
	if err != nil {
		log.Println(err)
		return "cli-v0.0.0"
	}

	return r.TagName
}

func refreshCLIVersionLoop() {
	ticker := time.NewTicker(5 * time.Minute)

	defer ticker.Stop()

	for range ticker.C {
		version := fetchLatestCLIVersion()

		if version != "" {
			latestCLIVersion = version
			log.Println("updated latest cli version:", version)
		}
	}
}

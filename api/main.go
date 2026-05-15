package main

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"
	"text/template"

	"github.com/go-chi/chi/v5"
)

var (
	publicAPIURL string
	publicWSURL  string
)

func main() {
	apiPort := os.Getenv("API_PORT")

	publicAPIURL = os.Getenv("PUBLIC_API_URL")
	publicWSURL = os.Getenv("PUBLIC_WS_URL")

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
		"Room":   room,
		"ApiURL": publicAPIURL,
		"WsURL":  publicWSURL,
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
		"Room":   room,
		"ApiURL": publicAPIURL,
		"WsURL":  publicWSURL,
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
	version := os.Getenv("RELEASE_VERSION")

	url := fmt.Sprintf(
		"https://github.com/%s/releases/download/%s/%s",
		repo,
		version,
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

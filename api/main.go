package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
)

var (
	publicAPIURL string
	publicWSURL  string
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("no .env file found")
	}

	apiPort := os.Getenv("API_PORT")

	publicAPIURL = os.Getenv("PUBLIC_API_URL")
	publicWSURL = os.Getenv("PUBLIC_WS_URL")

	r := chi.NewRouter()

	// Linux/macOS bootstrap
	r.Get("/", createRoomHandler)
	r.Get("/{room}", joinRoomHandler)

	// Windows bootstrap
	r.Get("/win/{room}", windowsJoinHandler)

	// Binary downloads
	r.Get("/bin/{binary}", binaryHandler)

	addr := ":" + apiPort

	log.Println("api server running on", addr)

	err = http.ListenAndServe(addr, r)
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

	script := fmt.Sprintf(`
$arch = $env:PROCESSOR_ARCHITECTURE

if ($arch -eq "AMD64") {
    $binary = "termchat-windows-amd64.exe"
} elseif ($arch -eq "ARM64") {
    $binary = "termchat-windows-arm64.exe"
} else {
    Write-Host "Unsupported architecture"
    exit
}

$temp = "$env:TEMP\termchat.exe"

Write-Host "Downloading $binary..."

Invoke-WebRequest -Uri "%s/bin/$binary" -OutFile $temp

Write-Host "Launching room %s..."

Start-Process -FilePath $temp -ArgumentList "--room %s --server %s"
`,
		publicAPIURL,
		room,
		room,
		publicWSURL,
	)

	w.Header().Set("Content-Type", "text/plain")

	w.Write([]byte(script))
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

func renderBootstrapScript(w http.ResponseWriter, room string) {
	script := fmt.Sprintf(`#!/bin/bash

OS=$(uname -s)
ARCH=$(uname -m)

case "$OS" in
    Linux)
        PLATFORM="linux"
        ;;
    Darwin)
        PLATFORM="darwin"
        ;;
    *)
        echo "Unsupported OS"
        exit 1
        ;;
esac

case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    arm64|aarch64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture"
        exit 1
        ;;
esac

BINARY="termchat-$PLATFORM-$ARCH"

TMP=$(mktemp)

echo "Downloading $BINARY..."

curl -fsSL %s/bin/$BINARY -o $TMP

chmod +x $TMP

echo "Launching room %s..."

$TMP --room %s --server %s
`,
		publicAPIURL,
		room,
		room,
		publicWSURL,
	)

	w.Header().Set("Content-Type", "text/plain")

	w.Write([]byte(script))
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

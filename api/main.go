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
	binaryPath   string
)

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Println("no .env file found")
	}

	apiPort := os.Getenv("API_PORT")

	publicAPIURL = os.Getenv("PUBLIC_API_URL")
	publicWSURL = os.Getenv("PUBLIC_WS_URL")

	binaryPath = os.Getenv("BINARY_PATH")

	if binaryPath == "" {
		binaryPath = "../dist/termchat"
	}

	r := chi.NewRouter()

	r.Get("/", createRoomHandler)
	r.Get("/{room}", joinRoomHandler)

	// binary download endpoint
	r.Get("/bin/termchat", binaryHandler)

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

func binaryHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, binaryPath)
}

func renderBootstrapScript(w http.ResponseWriter, room string) {
	script := fmt.Sprintf(`#!/bin/bash

TMP=$(mktemp)

echo "Downloading termchat..."

curl -sSL %s/bin/termchat -o $TMP

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

package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"termchat/shared"
)

type (
	Message  = shared.Message
	UserInfo = shared.UserInfo
)

var logger = log.New(os.Stderr, "", log.LstdFlags)

var initialPassword string

// SetInitialPassword sets a password that will be applied to the first room
// created on this server instance (used by `termchat host --password`).
func SetInitialPassword(password string) {
	initialPassword = password
}

func SetLogOutput(w io.Writer) {
	if w == nil {
		w = io.Discard
	}

	logger.SetOutput(w)
}

func StartServer(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWebSocket)
	mux.HandleFunc("/discover", handleDiscover)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Handle graceful shutdown on SIGTERM/SIGINT
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		<-sigCh

		logger.Println("shutdown signal received, closing connections")

		// Close all client connections gracefully
		roomsMutex.RLock()
		roomsCopy := make([]*Room, 0, len(rooms))
		for _, room := range rooms {
			roomsCopy = append(roomsCopy, room)
		}
		roomsMutex.RUnlock()

		for _, room := range roomsCopy {
			room.Mutex.Lock()
			for client := range room.Clients {
				// Notify client of shutdown
				select {
				case client.Send <- Message{
					Type: "system",
					Text: "server shutting down",
				}:
				default:
				}
				// Close connection
				client.Conn.Close()
			}
			room.Mutex.Unlock()
		}

		server.Close()
	}()

	logger.Println("websocket server running on", addr)

	go cleanupIdleClients()
	go cleanupTypingIndicators()

	return server.ListenAndServe()
}

func handleDiscover(w http.ResponseWriter, r *http.Request) {
	var roomList []shared.RoomInfo

	roomsMutex.RLock()
	roomsCopy := make([]*Room, 0, len(rooms))
	for _, room := range rooms {
		roomsCopy = append(roomsCopy, room)
	}
	roomsMutex.RUnlock()

	for _, room := range roomsCopy {
		room.Mutex.Lock()

		hostNick := ""
		if room.Host != nil {
			hostNick = room.Host.Nickname
		}

		info := shared.RoomInfo{
			ID:          room.ID,
			UserCount:   len(room.Clients),
			HasPassword: room.Password != "",
			HostNick:    hostNick,
		}

		room.Mutex.Unlock()

		roomList = append(roomList, info)
	}

	if roomList == nil {
		roomList = []shared.RoomInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roomList)
}

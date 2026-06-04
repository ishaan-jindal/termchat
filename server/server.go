package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

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

	logger.Println("websocket server running on", addr)

	go cleanupIdleClients()
	go cleanupTypingIndicators()

	return http.ListenAndServe(addr, mux)
}

func handleDiscover(w http.ResponseWriter, r *http.Request) {
	var roomList []shared.RoomInfo

	for _, room := range rooms {
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

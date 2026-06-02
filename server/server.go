package server

import (
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

func SetLogOutput(w io.Writer) {
	if w == nil {
		w = io.Discard
	}

	logger.SetOutput(w)
}

func StartServer(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleWebSocket)

	logger.Println("websocket server running on", addr)

	go cleanupIdleClients()
	go cleanupTypingIndicators()

	return http.ListenAndServe(addr, mux)
}

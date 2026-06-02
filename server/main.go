package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"termchat/shared"
)

type (
	Message  = shared.Message
	UserInfo = shared.UserInfo
)

func main() {
	host := os.Getenv("WS_HOST")
	port := os.Getenv("WS_PORT")

	addr := fmt.Sprintf("%s:%s", host, port)

	http.HandleFunc("/ws", handleWebSocket)

	log.Println("websocket server running on", addr)

	go cleanupIdleClients()
	go cleanupTypingIndicators()

	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

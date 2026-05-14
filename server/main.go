package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/joho/godotenv"
)

func main() {
	err := godotenv.Load(".env")
	if err != nil {
		log.Println("no .env file found")
	}

	host := os.Getenv("WS_HOST")
	port := os.Getenv("WS_PORT")

	addr := fmt.Sprintf("%s:%s", host, port)

	http.HandleFunc("/ws", handleWebSocket)

	log.Println("websocket server running on", addr)

	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal(err)
	}
}

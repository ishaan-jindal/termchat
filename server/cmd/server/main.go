package main

import (
	"fmt"
	"log"
	"os"

	"termchat/server"
)

func main() {
	host := os.Getenv("WS_HOST")
	port := os.Getenv("WS_PORT")
	if port == "" {
		port = "8080"
	}

	addr := fmt.Sprintf("%s:%s", host, port)

	log.Fatal(server.StartServer(addr))
}

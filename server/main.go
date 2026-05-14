package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/ws", handleWebSocket)

	log.Println("server running on :8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Nickname: ")
	nick, _ := reader.ReadString('\n')
	nick = strings.TrimSpace(nick)

	fmt.Print("Room: ")
	room, _ := reader.ReadString('\n')
	room = strings.TrimSpace(room)

	conn, err := connectWebSocket("ws://localhost:8080/ws")
	if err != nil {
		log.Fatal(err)
	}

	// Send join packet
	err = conn.WriteJSON(Message{
		Type: "join",
		Nick: nick,
		Room: room,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Connected to room", room)

	// Start message reader
	go readMessages(conn)

	// Input loop
	for {
		fmt.Print("> ")

		text, _ := reader.ReadString('\n')
		text = strings.TrimSpace(text)

		if text == "" {
			continue
		}

		err := conn.WriteJSON(Message{
			Type: "message",
			Text: text,
		})
		if err != nil {
			log.Println("failed to send message")
			return
		}
	}
}

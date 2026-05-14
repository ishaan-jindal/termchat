package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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

	err = conn.conn.WriteJSON(Message{
		Type: "join",
		Nick: nick,
		Room: room,
	})
	if err != nil {
		log.Fatal(err)
	}

	model := NewModel(conn, nick, room)

	p := tea.NewProgram(model)

	_, err = p.Run()
	if err != nil {
		log.Fatal(err)
	}

	conn.conn.Close()
}

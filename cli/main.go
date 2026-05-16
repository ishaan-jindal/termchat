package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

var Version = "dev"

func main() {
	reader := getInputReader()

	versionFlag := flag.Bool("version", false, "show version")
	roomFlag := flag.String("room", "", "room code")
	serverFlag := flag.String("server", "ws://localhost:8080/ws", "websocket server")

	flag.Parse()

	if *versionFlag {
		fmt.Println("termchat", Version)
		return
	}

	fmt.Print("Nickname: ")
	nick, _ := reader.ReadString('\n')
	nick = strings.TrimSpace(nick)

	room := *roomFlag

	if room == "" {
		fmt.Print("Room: ")
		roomInput, _ := reader.ReadString('\n')
		room = strings.TrimSpace(roomInput)
	}

	conn, err := connectWebSocket(*serverFlag)
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

	p := tea.NewProgram(
		model,
		tea.WithMouseCellMotion(),
	)

	_, err = p.Run()
	if err != nil {
		log.Fatal(err)
	}

	conn.conn.Close()
}

func getInputReader() *bufio.Reader {
	tty, err := os.Open("/dev/tty")
	if err == nil {
		return bufio.NewReader(tty)
	}

	return bufio.NewReader(os.Stdin)
}

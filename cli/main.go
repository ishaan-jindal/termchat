package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

var (
	Version    = "dev"
	DefaultAPI = "https://localhost"
	DefaultWS  = "ws://localhost:8080/ws"
)

func main() {
	reader := getInputReader()

	versionFlag := flag.Bool("version", false, "show version")
	roomFlag := flag.String("room", "", "room code")
	serverFlag := flag.String("server", DefaultWS, "websocket server")
	apiFlag := flag.String("api", DefaultAPI, "api server")

	flag.Parse()

	if *versionFlag {
		fmt.Println("termchat", Version)
		return
	}

	fmt.Print("Nickname: ")
	nick, _ := reader.ReadString('\n')
	nick = strings.TrimSpace(nick)

	var room string

	room = *roomFlag
	if flag.NArg() > 0 {
		room = flag.Arg(0)
	}

	if room == "" {
		room = fetchNewRoom(*apiFlag)
		fmt.Println("Created Room:", room)
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

func fetchNewRoom(apiURL string) string {
	resp, err := http.Get(apiURL + "/api/new")
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimSpace(string(body))
}

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

	"termchat/shared"

	tea "github.com/charmbracelet/bubbletea"
)

type (
	Message  = shared.Message
	UserInfo = shared.UserInfo
)

var (
	Version    = "dev"
	DefaultAPI = "https://termchat.sacred99.online"
	DefaultWS  = "wss://termchat.sacred99.online/ws"
)

func main() {
	cfg := loadConfig()

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

	if cfg.Nick != "" {
		fmt.Printf("Nickname [%s]: ", cfg.Nick)
	} else {
		fmt.Print("Nickname: ")
	}
	nick, _ := reader.ReadString('\n')
	nick = strings.TrimSpace(nick)
	if nick == "" {
		nick = cfg.Nick
	}
	if nick == "" {
		nick = "anonymous"
	}

	cfg.Nick = nick
	saveConfig(cfg)

	var room string

	room = *roomFlag
	if flag.NArg() > 0 {
		room = flag.Arg(0)
	}

	if room == "" {
		room = fetchNewRoom(*apiFlag)
		fmt.Println("Created Room:", room)
	}
	room = shared.NormalizeRoomCode(room)
	if !shared.IsValidRoomCode(room) {
		log.Fatalf("invalid room code %q", room)
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
	if cfg.Color != "" {
		conn.conn.WriteJSON(Message{
			Type:  "color",
			Color: cfg.Color,
		})
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

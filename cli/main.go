package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	chatserver "termchat/server"
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

const defaultLANPort = 8080

type cliOptions struct {
	Version bool
	Help    bool
	Room    string

	Server    string
	ServerSet bool

	API  string
	Host string
	Port int

	HostMode bool
}

func main() {
	opts, err := parseArgs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}

	if opts.Version {
		fmt.Println("termchat", Version)
		return
	}

	if opts.Help {
		printUsage(os.Stdout)
		return
	}

	room := opts.Room
	var localServerErrs <-chan error
	if opts.HostMode {
		room = prepareHostRoom(room)
		localServerErrs, err = startLocalServer(opts.Port)
		if err != nil {
			log.Fatal(err)
		}
	} else if room == "" {
		room = fetchNewRoom(opts.API)
		fmt.Println("Created Room:", room)
	}

	room = shared.NormalizeRoomCode(room)
	if !shared.IsValidRoomCode(room) {
		log.Fatalf("invalid room code %q", room)
	}

	cfg := loadConfig()
	reader := getInputReader()

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

	serverURL := websocketURL(opts)
	var conn *Connection

	if opts.HostMode {
		conn, err = connectLocalWebSocket(serverURL, localServerErrs)
	} else {
		conn, err = connectWebSocket(serverURL)
	}
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
	if opts.HostMode {
		model.IsHost = true
		model.HostIP = GetLocalIP()
		model.HostPort = opts.Port
	}

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

func parseArgs(args []string) (cliOptions, error) {
	opts := cliOptions{
		Server: DefaultWS,
		API:    DefaultAPI,
		Port:   defaultLANPort,
	}

	var positionals []string

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "" {
			continue
		}

		if !strings.HasPrefix(arg, "-") {
			if len(positionals) == 0 && arg == "host" {
				opts.HostMode = true
				continue
			}

			positionals = append(positionals, arg)
			continue
		}

		name, value, hasValue := splitFlag(arg)

		switch name {
		case "help", "h":
			opts.Help = true

		case "version":
			opts.Version = true

		case "room":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--room requires a value")
				}
				value = args[i]
			}
			opts.Room = value

		case "server":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--server requires a value")
				}
				value = args[i]
			}
			opts.Server = value
			opts.ServerSet = true

		case "api":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--api requires a value")
				}
				value = args[i]
			}
			opts.API = value

		case "host":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--host requires a value")
				}
				value = args[i]
			}
			opts.Host = value

		case "port":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--port requires a value")
				}
				value = args[i]
			}

			port, err := strconv.Atoi(value)
			if err != nil || port < 1 || port > 65535 {
				return opts, fmt.Errorf("invalid port %q", value)
			}
			opts.Port = port

		default:
			return opts, fmt.Errorf("unknown flag %s", arg)
		}
	}

	if opts.Room == "" && len(positionals) > 0 {
		opts.Room = positionals[0]
	}

	if len(positionals) > 1 {
		return opts, fmt.Errorf("unexpected argument %q", positionals[1])
	}

	return opts, nil
}

func printUsage(w io.Writer) {
	fmt.Fprintf(w, `Usage:
  termchat [options] [ROOM]
  termchat host [ROOM] [options]

Cloud rooms:
  termchat
  termchat FROG
  termchat --room FROG

LAN host mode:
  termchat host
  termchat host FROG
  termchat host --port 9000
  termchat host FROG --port 9000

LAN join:
  termchat FROG --host 192.168.1.42
  termchat FROG --host 192.168.1.42 --port 9000

Options:
  --room CODE       Join an existing room by code
  --host ADDRESS   Connect to a LAN host by IP or hostname
  --port PORT      LAN websocket port (default: %d)
  --server URL     WebSocket server URL (default: %s)
  --api URL        API server URL (default: %s)
  --version        Show version and exit
  --help, -h       Show this help and exit
`, defaultLANPort, DefaultWS, DefaultAPI)
}

func splitFlag(arg string) (name string, value string, hasValue bool) {
	name = strings.TrimLeft(arg, "-")

	if idx := strings.Index(name, "="); idx >= 0 {
		return name[:idx], name[idx+1:], true
	}

	return name, "", false
}

func prepareHostRoom(room string) string {
	if room == "" {
		return shared.GenerateRoomCode()
	}

	room = shared.NormalizeRoomCode(room)
	if !shared.IsValidRoomCode(room) {
		log.Fatalf("invalid room code %q", room)
	}

	return room
}

func startLocalServer(port int) (<-chan error, error) {
	chatserver.SetLogOutput(io.Discard)

	errCh := make(chan error, 1)
	go func() {
		errCh <- chatserver.StartServer(fmt.Sprintf(":%d", port))
	}()

	select {
	case err := <-errCh:
		return nil, err
	case <-time.After(50 * time.Millisecond):
		return errCh, nil
	}
}

func websocketURL(opts cliOptions) string {
	if opts.HostMode {
		return fmt.Sprintf("ws://localhost:%d/ws", opts.Port)
	}

	if opts.ServerSet {
		return opts.Server
	}

	if opts.Host != "" {
		return fmt.Sprintf("ws://%s:%d/ws", opts.Host, opts.Port)
	}

	return DefaultWS
}

func connectLocalWebSocket(serverURL string, serverErrs <-chan error) (*Connection, error) {
	deadline := time.Now().Add(3 * time.Second)

	for {
		select {
		case err := <-serverErrs:
			return nil, err
		default:
		}

		conn, err := connectWebSocket(serverURL)
		if err == nil {
			return conn, nil
		}

		if time.Now().After(deadline) {
			return nil, err
		}

		time.Sleep(50 * time.Millisecond)
	}
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

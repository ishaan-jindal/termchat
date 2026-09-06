package main

import (
	"errors"
	"fmt"
	"io"
	"log"
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
	Reaction = shared.Reaction
)

var (
	Version     = "dev"
	DefaultWS   = "wss://termchat.sacred99.online/ws"
	DefaultBase = "https://termchat.sacred99.online"
)

const defaultLANPort = 8080

type cliOptions struct {
	Version bool
	Help    bool
	Room    string

	Server    string
	ServerSet bool

	Host string
	Port int

	Password string

	Theme string

	HostMode     bool
	DiscoverMode bool
	OnlineOnly   bool
	LocalOnly    bool
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

	if opts.DiscoverMode && !stdoutIsTTY() {
		runDiscover(discoverOptions{
			Online: opts.OnlineOnly,
			Local:  opts.LocalOnly,
			Base:   discoverBaseURL(opts),
		})
		return
	}

	room := opts.Room
	fresh := false

	if opts.HostMode {
		room = prepareHostRoom(room)
	} else {
		room = shared.NormalizeRoomCode(room)
		if room != "" && !shared.IsValidRoomCode(room) {
			log.Fatalf("invalid room code %q", room)
		}

		if room == "" && !opts.DiscoverMode {
			room = shared.GenerateRoomCode()
			fresh = true
		}
	}

	cfg := loadConfig()

	themeName := opts.Theme
	if themeName == "" {
		themeName = cfg.Theme
	}
	if themeName == "" {
		themeName = "system"
	}

	theme, err := resolveTheme(themeName)
	if err != nil {
		log.Fatal(err)
	}

	if opts.Theme != "" && cfg.Theme != opts.Theme {
		cfg.Theme = opts.Theme
		saveConfig(cfg)
	}

	app := newAppModel(hubOptions{
		room:       room,
		fresh:      fresh,
		password:   opts.Password,
		serverURL:  websocketURL(opts),
		base:       discoverBaseURL(opts),
		hostMode:   opts.HostMode,
		port:       opts.Port,
		showOnline: !opts.LocalOnly,
		showLocal:  !opts.OnlineOnly,
		cfg:        cfg,
		theme:      theme,
	})

	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	finalModel, err := p.Run()
	if err != nil {
		log.Fatal(err)
	}

	// The shell tracks the live connection across chat reconnects.
	var live *Connection

	if fm, ok := finalModel.(appModel); ok {
		live = fm.liveConn()
	}

	if live == nil {
		return
	}

	select {
	case <-live.done:
		// writePump already stopped
	default:
		close(live.done)
	}

	live.conn.Close()
}

// stdoutIsTTY reports whether plain table output would be seen by a human.
// Piped discover output keeps the legacy printed tables for scripts.
func stdoutIsTTY() bool {
	st, err := os.Stdout.Stat()
	if err != nil {
		return false
	}

	return st.Mode()&os.ModeCharDevice != 0
}

func parseArgs(args []string) (cliOptions, error) {
	opts := cliOptions{
		Server: DefaultWS,
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

			if len(positionals) == 0 && arg == "discover" {
				opts.DiscoverMode = true
				continue
			}

			positionals = append(positionals, arg)
			continue
		}

		name, value, hasValue := splitFlag(arg)

		switch name {
		case "help", "h":
			opts.Help = true

		case "version", "v":
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

		case "password":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--password requires a value")
				}
				value = args[i]
			}
			opts.Password = value

		case "theme":
			if !hasValue {
				i++
				if i >= len(args) {
					return opts, errors.New("--theme requires a value")
				}
				value = args[i]
			}

			if !isThemeName(value) {
				return opts, fmt.Errorf("unknown theme %q (valid: %s)", value, validThemes())
			}
			opts.Theme = value

		case "online":
			opts.OnlineOnly = true

		case "local":
			opts.LocalOnly = true

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
  termchat discover [--online] [--local]

Cloud rooms:
  termchat
  termchat FROG
  termchat --room FROG

LAN host mode:
  termchat host
  termchat host FROG
  termchat host --port 9000
  termchat host FROG --port 9000 --password secret

LAN join:
  termchat FROG --host 192.168.1.42
  termchat FROG --host 192.168.1.42 --port 9000
  termchat FROG --host 192.168.1.42 --password secret

Discover rooms:
  termchat discover              Show online + LAN rooms
  termchat discover --online     Show only online rooms
  termchat discover --local      Show only LAN rooms

Options:
  --room CODE       Join an existing room by code
  --host ADDRESS    Connect to a LAN host by IP or hostname
  --port PORT       LAN websocket port (default: %d)
  --password PASS   Room password (for hosting or joining)
  --server URL      WebSocket server URL (default: %s)
  --theme NAME      Color theme: %s (default: system)
  --online          Discover: show only online rooms
  --local           Discover: show only LAN rooms
  --version, -v         Show version and exit
  --help, -h        Show this help and exit
`, defaultLANPort, DefaultWS, validThemes())
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

func startLocalServer(port int, password string) (<-chan error, error) {
	chatserver.SetLogOutput(io.Discard)

	if password != "" {
		chatserver.SetInitialPassword(password)
	}

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

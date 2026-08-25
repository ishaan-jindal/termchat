package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"termchat/shared"
)

type (
	Message  = shared.Message
	UserInfo = shared.UserInfo
)

var logger = log.New(os.Stderr, "", log.LstdFlags)

var (
	initialPassword string

	stopMu  sync.Mutex
	stopCh  chan struct{}
	drainCh chan struct{}
)

// SetInitialPassword sets a password that will be applied to the first room
// created on this server instance (used by `termchat host --password`).
func SetInitialPassword(password string) {
	initialPassword = password
}

// Stop gracefully shuts down the server started by StartServer and blocks
// until it has fully drained (client connections closed, listener down), so
// a subsequent StartServer can safely swap the room registry. It is
// idempotent and safe to call before StartServer.
func Stop() {
	stopMu.Lock()
	c := stopCh
	d := drainCh
	stopMu.Unlock()

	if c == nil {
		return
	}

	select {
	case <-c:
	default:
		close(c)
	}

	if d != nil {
		<-d
	}
}

func SetLogOutput(w io.Writer) {
	if w == nil {
		w = io.Discard
	}

	logger.SetOutput(w)
}

func newMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /ws", handleWebSocket)
	mux.HandleFunc("GET /discover", handleDiscover)

	// Bootstrap scripts (one-liner install flow)
	mux.HandleFunc("GET /{$}", createRoomHandler)
	mux.HandleFunc("GET /{room}", joinRoomHandler)
	mux.HandleFunc("GET /win", windowsCreateRoomHandler)
	mux.HandleFunc("GET /win/{room}", windowsJoinHandler)
	mux.HandleFunc("GET /bin/{binary}", binaryHandler)

	return mux
}

func StartServer(addr string) error {
	initBootstrapConfig()

	// A server instance owns a fresh room registry. The map is package-global,
	// so a new server must not inherit rooms from a previous instance. This is
	// safe because Stop() blocks until the previous server has fully drained:
	// its clients are disconnected and its rooms are gone, so nothing live is
	// ever orphaned by the swap.
	roomsMutex.Lock()
	rooms = map[string]*Room{}
	roomsMutex.Unlock()

	resetMediaTokens()

	stopMu.Lock()
	stopCh = make(chan struct{})
	drainCh = make(chan struct{})
	c := stopCh
	d := drainCh
	stopMu.Unlock()

	server := &http.Server{
		Addr:    addr,
		Handler: newMux(),
	}

	// Handle graceful shutdown on SIGTERM/SIGINT or Stop()
	go func() {
		defer close(d)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

		select {
		case <-sigCh:
		case <-c:
		}

		logger.Println("shutdown signal received, closing connections")

		// Close all client connections gracefully
		roomsMutex.RLock()
		roomsCopy := make([]*Room, 0, len(rooms))
		for _, room := range rooms {
			roomsCopy = append(roomsCopy, room)
		}
		roomsMutex.RUnlock()

		for _, room := range roomsCopy {
			room.Mutex.Lock()
			for client := range room.Clients {
				client.trySend(Message{
					Type: "system",
					Text: "server shutting down",
				})
			}
			room.Mutex.Unlock()
		}

		// Let writePump flush the shutdown notice before closing conns;
		// closing immediately races the pending write and loses it.
		time.Sleep(200 * time.Millisecond)

		for _, room := range roomsCopy {
			room.Mutex.Lock()
			for client := range room.Clients {
				// Close connection
				client.Conn.Close()
			}
			room.Mutex.Unlock()
		}

		server.Close()
	}()

	logger.Println("websocket server running on", addr)

	go refreshCLIVersionLoop(c)
	go cleanupIdleClients(c)
	go cleanupTypingIndicators(c)

	return server.ListenAndServe()
}

func handleDiscover(w http.ResponseWriter, r *http.Request) {
	var roomList []shared.RoomInfo

	roomsMutex.RLock()
	roomsCopy := make([]*Room, 0, len(rooms))
	for _, room := range rooms {
		roomsCopy = append(roomsCopy, room)
	}
	roomsMutex.RUnlock()

	for _, room := range roomsCopy {
		room.Mutex.Lock()

		hostNick := ""
		if room.Host != nil {
			hostNick = room.Host.nickname()
		}

		info := shared.RoomInfo{
			ID:          room.ID,
			UserCount:   len(room.Clients),
			HasPassword: room.Password != "",
			HostNick:    hostNick,
		}

		room.Mutex.Unlock()

		roomList = append(roomList, info)
	}

	if roomList == nil {
		roomList = []shared.RoomInfo{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roomList)
}

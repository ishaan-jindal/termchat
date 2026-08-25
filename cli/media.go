package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

// VoiceSession is the client's joined voice state; nil while not in voice.
type VoiceSession struct {
	conn *MediaConn
}

// MediaConn is the client side of the binary /media WebSocket.
type MediaConn struct {
	conn      *websocket.Conn
	outbox    chan []byte
	inbox     chan []byte
	done      chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
}

func (mc *MediaConn) close() {
	mc.closeOnce.Do(func() {
		close(mc.done)
		mc.conn.Close()
	})
}

// dialMedia performs the /media join handshake and starts both pumps.
func dialMedia(base, room, token string) (*MediaConn, error) {
	url := strings.TrimSuffix(base, "/") + "/media"

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("dialing voice endpoint: %w", err)
	}

	err = conn.WriteJSON(Message{Type: "join", Room: room, Token: token})
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("joining voice session: %w", err)
	}

	var reply Message

	err = conn.ReadJSON(&reply)
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("reading voice reply: %w", err)
	}

	if reply.Type == "error" {
		text := reply.Text
		if text == "" {
			text = "unknown error"
		}

		conn.Close()

		return nil, fmt.Errorf("voice join rejected: %s", text)
	}

	if reply.Type != "ok" {
		conn.Close()

		return nil, fmt.Errorf("unexpected voice reply %q", reply.Type)
	}

	mc := &MediaConn{
		conn:   conn,
		outbox: make(chan []byte, 8),
		inbox:  make(chan []byte, 64),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}

	go mc.readLoop()
	go mc.writeLoop()

	return mc, nil
}

func (mc *MediaConn) readLoop() {
	defer close(mc.closed)

	for {
		msgType, frame, err := mc.conn.ReadMessage()
		if err != nil {
			return
		}

		if msgType != websocket.BinaryMessage {
			continue
		}

		select {
		case mc.inbox <- frame:
		default:
		}
	}
}

func (mc *MediaConn) writeLoop() {
	for {
		select {
		case frame := <-mc.outbox:
			err := mc.conn.WriteMessage(websocket.BinaryMessage, frame)
			if err != nil {
				mc.close()

				return
			}

		case <-mc.done:
			return
		}
	}
}

type voiceReadyMsg struct{ conn *MediaConn }

type voiceErrorMsg struct{ err error }

type voiceEndedMsg struct{}

type voiceTimeoutTickMsg struct{}

func dialMediaCmd(base, room, token string) tea.Cmd {
	return func() tea.Msg {
		mc, err := dialMedia(base, room, token)

		if err != nil {
			return voiceErrorMsg{err: err}
		}

		return voiceReadyMsg{conn: mc}
	}
}

func waitForVoiceEnd(mc *MediaConn) tea.Cmd {
	return func() tea.Msg {
		<-mc.closed

		return voiceEndedMsg{}
	}
}

func voiceTimeoutCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return voiceTimeoutTickMsg{}
	})
}

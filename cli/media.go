package main

import (
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"

	"termchat/shared"
)

// MediaConn is the client side of the binary /media WebSocket. One conn
// carries both voice and video; readLoop routes frames by kind so either
// session can be absent without starving the other.
type MediaConn struct {
	conn      *websocket.Conn
	outbox    chan []byte
	audio     chan []byte
	video     chan []byte
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
		return nil, fmt.Errorf("dialing media endpoint: %w", err)
	}

	err = conn.WriteJSON(Message{Type: "join", Room: room, Token: token})
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("joining media session: %w", err)
	}

	var reply Message

	err = conn.ReadJSON(&reply)
	if err != nil {
		conn.Close()

		return nil, fmt.Errorf("reading media reply: %w", err)
	}

	if reply.Type == "error" {
		text := reply.Text
		if text == "" {
			text = "unknown error"
		}

		conn.Close()

		return nil, fmt.Errorf("media join rejected: %s", text)
	}

	if reply.Type != "ok" {
		conn.Close()

		return nil, fmt.Errorf("unexpected media reply %q", reply.Type)
	}

	mc := &MediaConn{
		conn:   conn,
		outbox: make(chan []byte, 8),
		audio:  make(chan []byte, 64),
		video:  make(chan []byte, 64),
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

		kind, _, _, _, ok := shared.ParseMediaFrame(frame)
		if !ok {
			continue
		}

		var inbox chan []byte

		switch kind {
		case shared.MediaKindAudio:
			inbox = mc.audio
		case shared.MediaKindVideo:
			inbox = mc.video
		default:
			continue
		}

		select {
		case inbox <- frame:
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

// trySend enqueues an outbound frame without blocking; frames are droppable
// because the next capture chunk supersedes the lost one.
func (mc *MediaConn) trySend(frame []byte) bool {
	select {
	case mc.outbox <- frame:
		return true
	case <-mc.done:
		return false
	default:
		return false
	}
}

type mediaReadyMsg struct{ conn *MediaConn }

type mediaErrorMsg struct{ err error }

type mediaEndedMsg struct{}

type mediaTimeoutTickMsg struct{}

func dialMediaCmd(base, room, token string) tea.Cmd {
	return func() tea.Msg {
		mc, err := dialMedia(base, room, token)

		if err != nil {
			return mediaErrorMsg{err: err}
		}

		return mediaReadyMsg{conn: mc}
	}
}

func waitForMediaEnd(mc *MediaConn) tea.Cmd {
	return func() tea.Msg {
		<-mc.closed

		return mediaEndedMsg{}
	}
}

func mediaTimeoutCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return mediaTimeoutTickMsg{}
	})
}

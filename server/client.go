package server

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	Conn   *websocket.Conn
	RoomID string
	Send   chan Message

	// mu guards all mutable client state below. Lock ordering: room.Mutex
	// may be held while acquiring client.mu, never the reverse.
	mu           sync.Mutex
	Nickname     string
	Color        string
	Typing       bool
	LastTyping   time.Time
	LastActivity time.Time
	JoinedAt     time.Time
	VoiceID      uint32

	// MessageTimestamps is only accessed by readPump, no lock needed.
	MessageTimestamps []time.Time

	done      chan struct{}
	closeOnce sync.Once
}

func newClient(conn *websocket.Conn) *Client {
	return &Client{
		Conn:         conn,
		Send:         make(chan Message, 32),
		JoinedAt:     time.Now(),
		LastActivity: time.Now(),
		done:         make(chan struct{}),
	}
}

// close signals that the client is shutting down. It is idempotent and the
// only caller is cleanupClient. writePump and broadcasters observe done
// instead of relying on Send being closed, which avoids the race between
// channel close and concurrent sends.
func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
	})
}

func (c *Client) isDone() <-chan struct{} {
	return c.done
}

// trySend delivers msg to the client without blocking. It returns false when
// the client is shutting down or its outbound queue is full.
func (c *Client) trySend(msg Message) bool {
	select {
	case c.Send <- msg:
		return true
	case <-c.done:
		return false
	default:
		logger.Println("dropping message for slow client")
		return false
	}
}

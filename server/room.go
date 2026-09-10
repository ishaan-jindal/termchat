package server

import "sync"

// Room holds the clients, history, and reactions for one chat room.
type Room struct {
	ID       string
	Password string
	Host     *Client
	Clients  map[*Client]bool
	History  []Message

	// NextID is the next message ID to stamp, guarded by Mutex.
	NextID int64

	// Reactions maps message ID -> reaction name -> voter set, so a user can
	// toggle their own vote. Guarded by Mutex.
	Reactions map[int64]map[string]map[*Client]bool

	Mutex sync.Mutex
}

var (
	rooms      = map[string]*Room{}
	roomsMutex sync.RWMutex
)

package server

import "sync"

type Room struct {
	ID       string
	Password string
	Host     *Client
	Clients  map[*Client]bool
	History  []Message
	Mutex    sync.Mutex
}

var (
	rooms      = map[string]*Room{}
	roomsMutex sync.RWMutex
)

package server

import "sync"

type Room struct {
	ID      string
	Clients map[*Client]bool
	History []Message
	Mutex   sync.Mutex
}

var rooms = map[string]*Room{}

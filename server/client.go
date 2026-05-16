package main

import (
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	Conn     *websocket.Conn
	Nickname string
	RoomID   string
	Send     chan Message
	Color    string

	LastActivity      time.Time
	MessageTimestamps []time.Time
}

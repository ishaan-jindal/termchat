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

	Typing     bool
	LastTyping time.Time

	JoinedAt          time.Time
	LastActivity      time.Time
	MessageTimestamps []time.Time
}

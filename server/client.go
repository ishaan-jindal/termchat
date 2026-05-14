package main

import "github.com/gorilla/websocket"

type Client struct {
	Conn     *websocket.Conn
	Nickname string
	RoomID   string
	Send     chan Message
	Color    string
}

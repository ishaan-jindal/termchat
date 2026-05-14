package main

type Message struct {
	Type string `json:"type"`
	Nick string `json:"nick,omitempty"`
	Room string `json:"room,omitempty"`
	Text string `json:"text,omitempty"`
}

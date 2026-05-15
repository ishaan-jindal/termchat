package main

type Message struct {
	Type     string    `json:"type"`
	Nick     string    `json:"nick,omitempty"`
	Room     string    `json:"room,omitempty"`
	Text     string    `json:"text,omitempty"`
	NewNick  string    `json:"new_nick,omitempty"`
	Color    string    `json:"color,omitempty"`
	Messages []Message `json:"messages,omitempty"`
}

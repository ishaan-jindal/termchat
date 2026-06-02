package shared

type Message struct {
	Type     string     `json:"type"`
	Nick     string     `json:"nick,omitempty"`
	Room     string     `json:"room,omitempty"`
	Text     string     `json:"text,omitempty"`
	NewNick  string     `json:"new_nick,omitempty"`
	Color    string     `json:"color,omitempty"`
	Messages []Message  `json:"messages,omitempty"`
	Users    []UserInfo `json:"users,omitempty"`
}

type UserInfo struct {
	Nick     string `json:"nick"`
	Color    string `json:"color"`
	JoinedAt int64  `json:"joined_at"`
	Typing   bool   `json:"typing"`
}

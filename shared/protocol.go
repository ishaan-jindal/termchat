package shared

type Message struct {
	Type      string     `json:"type"`
	Nick      string     `json:"nick,omitempty"`
	Room      string     `json:"room,omitempty"`
	Text      string     `json:"text,omitempty"`
	NewNick   string     `json:"new_nick,omitempty"`
	Color     string     `json:"color,omitempty"`
	Password  string     `json:"password,omitempty"`
	Timestamp int64      `json:"timestamp,omitempty"`
	Messages  []Message  `json:"messages,omitempty"`
	Users     []UserInfo `json:"users,omitempty"`
}

type UserInfo struct {
	Nick     string `json:"nick"`
	Color    string `json:"color"`
	JoinedAt int64  `json:"joined_at"`
	Typing   bool   `json:"typing"`
	IsHost   bool   `json:"is_host"`
}

// RoomInfo is returned by the /discover HTTP endpoint.
type RoomInfo struct {
	ID          string `json:"id"`
	UserCount   int    `json:"user_count"`
	HasPassword bool   `json:"has_password"`
	HostNick    string `json:"host_nick"`
}

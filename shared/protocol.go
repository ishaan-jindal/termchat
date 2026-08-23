package shared

type Message struct {
	ID          int64      `json:"id"`
	Type        string     `json:"type"`
	Nick        string     `json:"nick,omitempty"`
	Room        string     `json:"room,omitempty"`
	Text        string     `json:"text,omitempty"`
	ReplyToID   int64      `json:"reply_to_id,omitempty"`
	ReplyToNick string     `json:"reply_to_nick,omitempty"`
	ReplyToText string     `json:"reply_to_text,omitempty"`
	Reactions   []Reaction `json:"reactions,omitempty"`
	NewNick     string     `json:"new_nick,omitempty"`
	Color       string     `json:"color,omitempty"`
	Password    string     `json:"password,omitempty"`
	Timestamp   int64      `json:"timestamp,omitempty"`

	// ServerTime is the server's current unix time, sent with users_list so
	// clients can correct for clock skew between the two machines.
	ServerTime int64      `json:"server_time,omitempty"`
	Messages   []Message  `json:"messages,omitempty"`
	Users      []UserInfo `json:"users,omitempty"`
}

type Reaction struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// ReactionNames is the fixed set of reaction names a client may send.
var ReactionNames = []string{"+1", "-1", "laugh", "heart", "wow", "eyes", "fire", "clap"}

func IsValidReaction(name string) bool {
	for _, n := range ReactionNames {
		if n == name {
			return true
		}
	}

	return false
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

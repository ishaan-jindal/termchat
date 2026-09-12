package shared

// Room code shape and LAN discovery constants.
const (
	RoomCodeLength  = 4
	RoomCodeCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	// LAN discovery constants
	DiscoveryPort      = 9999
	DiscoveryMulticast = "224.0.0.167"
	DiscoveryMagic     = "TERMCHAT_DISCOVER"
)

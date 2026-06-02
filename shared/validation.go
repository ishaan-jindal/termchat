package shared

import (
	"regexp"
	"strings"
)

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func IsValidHexColor(color string) bool {
	return hexColorRegex.MatchString(color)
}

func IsValidRoomCode(room string) bool {
	room = NormalizeRoomCode(room)
	if len(room) != RoomCodeLength {
		return false
	}

	for _, char := range room {
		if !strings.ContainsRune(RoomCodeCharset, char) {
			return false
		}
	}

	return true
}

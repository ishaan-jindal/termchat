package shared

import (
	"regexp"
	"strings"
)

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func IsValidHexColor(color string) bool {
	return hexColorRegex.MatchString(color)
}

// IsValidRoomCode reports whether room is a well-formed room code.
// The input must already be normalized (see NormalizeRoomCode); all callers
// normalize before validating.
func IsValidRoomCode(room string) bool {
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

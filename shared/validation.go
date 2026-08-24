package shared

import (
	"regexp"
	"strings"
)

// MaxNicknameLength caps nicknames in runes.
const MaxNicknameLength = 32

var hexColorRegex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func IsValidHexColor(color string) bool {
	return hexColorRegex.MatchString(color)
}

// IsValidNickname reports whether nick is a usable chat name: non-empty, at
// most MaxNicknameLength runes, and only printable ASCII with no spaces.
func IsValidNickname(nick string) bool {
	runes := []rune(nick)
	if len(runes) == 0 || len(runes) > MaxNicknameLength {
		return false
	}

	for _, r := range runes {
		if r < 0x21 || r > 0x7E {
			return false
		}
	}

	return true
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

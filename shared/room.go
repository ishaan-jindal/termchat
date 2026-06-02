package shared

import (
	"crypto/rand"
	"strings"
)

func GenerateRoomCode() string {
	bytes := make([]byte, RoomCodeLength)

	_, err := rand.Read(bytes)
	if err != nil {
		return "FROG"
	}

	result := make([]byte, RoomCodeLength)

	for i, b := range bytes {
		result[i] = RoomCodeCharset[int(b)%len(RoomCodeCharset)]
	}

	return string(result)
}

func NormalizeRoomCode(room string) string {
	return strings.ToUpper(strings.TrimSpace(room))
}

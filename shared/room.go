package shared

import (
	"crypto/rand"
	"strings"
)

func GenerateRoomCode() string {
	out := make([]byte, RoomCodeLength)

	for i := range out {
		for {
			var b [1]byte

			if _, err := rand.Read(b[:]); err != nil {
				return "FROG"
			}

			// Rejection sampling: with b in [0, 256), taking b % len(charset)
			// would bias the first 256%len(charset) characters. Accept only
			// values in a range divisible by the charset size.
			if int(b[0]) < 256-(256%len(RoomCodeCharset)) {
				out[i] = RoomCodeCharset[int(b[0])%len(RoomCodeCharset)]
				break
			}
		}
	}

	return string(out)
}

func NormalizeRoomCode(room string) string {
	return strings.ToUpper(strings.TrimSpace(room))
}

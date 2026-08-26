package shared

import "testing"

func FuzzRoomCodeValidation(f *testing.F) {
	f.Add("FROG")
	f.Add("frog")
	f.Add("")
	f.Add("FROGS")
	f.Add("AB!C")
	f.Add("A\u00e9BC")
	f.Add(stringsRepeatA(300))

	f.Fuzz(func(t *testing.T, code string) {
		valid := IsValidRoomCode(code)

		if valid && len(code) != RoomCodeLength {
			t.Fatalf("IsValidRoomCode(%q) = true but length is %d", code, len(code))
		}

		// A valid code is already normalized.
		if valid && NormalizeRoomCode(code) != code {
			t.Fatalf("valid code %q is not normalized", code)
		}
	})
}

func FuzzParseMediaFrame(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte{0x01})
	f.Add([]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x01})
	f.Add([]byte{0x02, 0xFF, 0xDE, 0xAD, 0xBE, 0xEF})

	f.Fuzz(func(t *testing.T, data []byte) {
		kind, codec, voiceID, payload, ok := ParseMediaFrame(data)

		if !ok {
			return
		}

		if kind != MediaKindAudio && kind != MediaKindVideo {
			t.Fatalf("ParseMediaFrame(%v) accepted unknown kind %#x", data, kind)
		}

		if len(payload) != len(data)-MediaHeaderLen {
			t.Fatalf("payload length %d, want %d", len(payload), len(data)-MediaHeaderLen)
		}

		frame := EncodeAudioFrame(kind, codec, voiceID, payload)

		if string(frame) != string(data) {
			t.Fatalf("roundtrip mismatch: %v != %v", frame, data)
		}
	})
}

func stringsRepeatA(n int) string {
	b := make([]byte, n)

	for i := range b {
		b[i] = 'A'
	}

	return string(b)
}

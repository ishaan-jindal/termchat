package shared

import "testing"

func TestGenerateRoomCode(t *testing.T) {
	seen := map[string]bool{}

	for i := 0; i < 1000; i++ {
		code := GenerateRoomCode()

		if !IsValidRoomCode(code) {
			t.Fatalf("GenerateRoomCode() = %q, want valid room code", code)
		}

		if seen[code] {
			t.Fatalf("GenerateRoomCode() returned duplicate %q in 1000 draws", code)
		}

		seen[code] = true
	}
}

func TestGenerateRoomCodeUniformity(t *testing.T) {
	// With 36^4 = 1.68M codes and 200k draws, each of the 36 chars should
	// appear roughly evenly across each position. Tolerances are generous
	// to avoid flakiness while still catching modulo-bias regressions.
	draws := 200_000
	counts := [RoomCodeLength][36]int{}

	for i := 0; i < draws; i++ {
		code := GenerateRoomCode()

		for pos := 0; pos < RoomCodeLength; pos++ {
			idx := indexOf(RoomCodeCharset, code[pos])
			if idx < 0 {
				t.Fatalf("GenerateRoomCode() = %q contains char not in charset", code)
			}
			counts[pos][idx]++
		}
	}

	expected := float64(draws) / 36
	lo := int(expected * 0.93)
	hi := int(expected * 1.07)

	for pos := 0; pos < RoomCodeLength; pos++ {
		for c := 0; c < 36; c++ {
			if counts[pos][c] < lo || counts[pos][c] > hi {
				t.Errorf(
					"position %d char %q count = %d, want between %d and %d",
					pos,
					RoomCodeCharset[c],
					counts[pos][c],
					lo,
					hi,
				)
			}
		}
	}
}

func indexOf(charset string, c byte) int {
	for i := 0; i < len(charset); i++ {
		if charset[i] == c {
			return i
		}
	}

	return -1
}

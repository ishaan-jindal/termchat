package shared

import (
	"strings"
	"testing"
)

func TestIsValidHexColor(t *testing.T) {
	valid := []string{
		"#000000",
		"#FFFFFF",
		"#ffffff",
		"#00d7ff",
		"#5f87ff",
	}

	for _, c := range valid {
		if !IsValidHexColor(c) {
			t.Errorf("IsValidHexColor(%q) = false, want true", c)
		}
	}

	invalid := []string{
		"",
		"#",
		"#FFF",
		"#GGGGGG",
		"#00000",
		"#0000000",
		"000000",
		"red",
		"#00000G",
		"# ff0000",
	}

	for _, c := range invalid {
		if IsValidHexColor(c) {
			t.Errorf("IsValidHexColor(%q) = true, want false", c)
		}
	}
}

func TestIsValidRoomCode(t *testing.T) {
	valid := []string{
		"FROG",
		"ABCD",
		"7WHB",
		"0000",
		"ZZZZ",
	}

	for _, code := range valid {
		if !IsValidRoomCode(code) {
			t.Errorf("IsValidRoomCode(%q) = false, want true", code)
		}
	}

	invalid := []string{
		"",
		"FRO",
		"FROGS",
		"frog",
		"FROG ",
		" FROG",
		"FRO!",
		"ab-c",
		"AB1_",
		"\u03c0\u03c0\u03c0\u03c0",
	}

	for _, code := range invalid {
		if IsValidRoomCode(code) {
			t.Errorf("IsValidRoomCode(%q) = true, want false", code)
		}
	}
}

func TestNormalizeRoomCode(t *testing.T) {
	cases := map[string]string{
		"frog":   "FROG",
		" Frog ": "FROG",
		"fRoG":   "FROG",
		"  abcd": "ABCD",
	}

	for in, want := range cases {
		if got := NormalizeRoomCode(in); got != want {
			t.Errorf("NormalizeRoomCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsValidNickname(t *testing.T) {
	valid := []string{
		"alice",
		"anonymous",
		"Bob_42",
		"n!c#-name",
		strings.Repeat("n", MaxNicknameLength),
	}

	for _, nick := range valid {
		if !IsValidNickname(nick) {
			t.Errorf("IsValidNickname(%q) = false, want true", nick)
		}
	}

	invalid := []string{
		"",
		strings.Repeat("n", MaxNicknameLength+1),
		"a b",
		" lead",
		"trail ",
		"tab\tinside",
		"h\u00e9llo",
		"\u65e5\u672c\u8a9e",
		"\U0001f600",
		"ctrl\x01char",
		"\u00a0nbsp",
	}

	for _, nick := range invalid {
		if IsValidNickname(nick) {
			t.Errorf("IsValidNickname(%q) = true, want false", nick)
		}
	}
}

package shared

import "testing"

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
		"ππππ",
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

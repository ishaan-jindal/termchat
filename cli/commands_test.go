package main

import (
	"strings"
	"testing"
)

func TestMoveSel(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)
	a.moveSel("down")
	if a.sel != 0 {
		t.Errorf("empty rows sel = %d, want 0", a.sel)
	}

	a.moveSel("up")
	if a.sel != 0 {
		t.Errorf("empty rows sel = %d, want 0", a.sel)
	}

	a.rows = []hubRow{{room: "A"}, {room: "B"}, {room: "C"}}

	a.sel = 0
	a.moveSel("up")
	if a.sel != 2 {
		t.Errorf("wrap up sel = %d, want 2", a.sel)
	}

	a.sel = 2
	a.moveSel("down")
	if a.sel != 0 {
		t.Errorf("wrap down sel = %d, want 0", a.sel)
	}

	a.sel = 1
	a.moveSel("down")
	if a.sel != 2 {
		t.Errorf("step down sel = %d, want 2", a.sel)
	}

	a.sel = 1
	a.moveSel("up")
	if a.sel != 0 {
		t.Errorf("step up sel = %d, want 0", a.sel)
	}
}

func testUsers() []UserInfo {
	return []UserInfo{
		{Nick: "alice", Color: "#ff0000"},
		{Nick: "bob", Color: "#00ff00"},
		{Nick: "aldo", Color: "#0000ff"},
	}
}

func TestMentionSuggestions(t *testing.T) {
	users := testUsers()

	got := mentionSuggestions("", users, "alice")
	if len(got) != 2 {
		t.Fatalf("empty query got %d rows, want 2 (self skipped)", len(got))
	}
	for _, s := range got {
		if strings.Contains(s.primary, "alice") {
			t.Errorf("self leaked into suggestions: %+v", got)
		}
		if !strings.HasPrefix(s.insert, "@") || !strings.HasSuffix(s.insert, " ") {
			t.Errorf("insert = %q, want @nick + space", s.insert)
		}
	}

	got = mentionSuggestions("AL", users, "bob")
	if len(got) != 2 {
		t.Fatalf("prefix AL got %d rows, want 2", len(got))
	}

	got = mentionSuggestions("zzz", users, "nobody")
	if len(got) != 0 {
		t.Errorf("no-match got %d rows, want 0", len(got))
	}
}

func TestWhisperSuggestions(t *testing.T) {
	users := testUsers()

	plain := whisperSuggestions("al", users, "bob")
	if len(plain) != 2 {
		t.Fatalf("plain query got %d rows, want 2", len(plain))
	}

	// Leading @ is stripped before matching.
	ated := whisperSuggestions("@al", users, "bob")
	if len(ated) != len(plain) {
		t.Fatalf("@ query got %d rows, want %d", len(ated), len(plain))
	}

	self := whisperSuggestions("", users, "alice")
	for _, s := range self {
		if s.primary == "@alice" {
			t.Errorf("self leaked into whisper suggestions: %+v", self)
		}
	}

	none := whisperSuggestions("zzz", users, "nobody")
	if len(none) != 0 {
		t.Errorf("no-match got %d rows, want 0", len(none))
	}
}

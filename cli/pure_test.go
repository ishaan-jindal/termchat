package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"termchat/shared"
)

func TestFirstLine(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"single", "boom", "boom"},
		{"empty", "", ""},
		{"multi", "a\nb\nc", "a"},
		{"trailing newline", "a\n", "a"},
		{"leading newline", "\nfoo", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := firstLine(c.in)
			if got != c.want {
				t.Errorf("firstLine(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestNextBackoff(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want time.Duration
	}{
		{"doubles", time.Second, 2 * time.Second},
		{"caps at max", 15 * time.Second, 30 * time.Second},
		{"over max stays", 20 * time.Second, 30 * time.Second},
		{"at max stays", 30 * time.Second, 30 * time.Second},
		{"way over max", 100 * time.Second, 30 * time.Second},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nextBackoff(c.in)
			if got != c.want {
				t.Errorf("nextBackoff(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestTileCaption(t *testing.T) {
	cases := []struct {
		name string
		nick string
		w    int
		want string
	}{
		{"pads", "alice", 10, " alice    "},
		{"exact", "ab", 3, " ab"},
		{"truncates", "averylongnick", 5, " aver"},
		{"empty nick pads", "", 4, "    "},
		{"zero width", "bob", 0, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := tileCaption(c.nick, c.w)
			if got != c.want {
				t.Errorf("tileCaption(%q, %d) = %q, want %q", c.nick, c.w, got, c.want)
			}
			if n := len([]rune(got)); n != c.w {
				t.Errorf("width = %d, want %d", n, c.w)
			}
		})
	}
}

func TestPrintUsage(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	out := buf.String()

	for _, want := range []string{"Usage:", "--room", "--port", "--theme", "termchat"} {
		if !strings.Contains(out, want) {
			t.Errorf("usage missing %q:\n%s", want, out)
		}
	}
}

func TestRescanClears(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)
	a.online = []shared.RoomInfo{{ID: "ABCD"}}
	a.onlineErr = "boom"
	a.onlineDone = true
	a.lan = []lanBeacon{{Room: "IJKL"}}
	a.lanDone = true
	a.rows = []hubRow{{room: "ABCD"}}
	a.sel = 2

	_ = a.rescan()

	if a.online != nil {
		t.Errorf("online not cleared: %+v", a.online)
	}
	if a.onlineErr != "" {
		t.Errorf("onlineErr = %q, want empty", a.onlineErr)
	}
	if a.onlineDone {
		t.Error("onlineDone not cleared")
	}
	if a.lan != nil {
		t.Errorf("lan not cleared: %+v", a.lan)
	}
	if a.lanDone {
		t.Error("lanDone not cleared")
	}
	if a.rows != nil {
		t.Errorf("rows not cleared: %+v", a.rows)
	}
	if a.sel != 0 {
		t.Errorf("sel = %d, want 0", a.sel)
	}
}

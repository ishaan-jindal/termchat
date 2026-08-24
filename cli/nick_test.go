package main

import (
	"bufio"
	"strings"
	"testing"

	"termchat/shared"
)

func TestPromptNicknameRetriesInvalid(t *testing.T) {
	input := "bad name\nn\u00e9ck\n" + strings.Repeat("x", 40) + "\n  robert  \n"

	var out strings.Builder

	nick := promptNickname(bufio.NewReader(strings.NewReader(input)), &out, "")

	if nick != "robert" {
		t.Fatalf("nick = %q, want robert", nick)
	}

	rendered := out.String()

	for _, want := range []string{"spaces are not allowed", "printable ASCII", "at most 32"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("output missing %q: %q", want, rendered)
		}
	}
}

func TestPromptNicknameDefaults(t *testing.T) {
	cases := []struct {
		name  string
		saved string
		input string
		want  string
	}{
		{"blank keeps valid saved", "alice", "\n", "alice"},
		{"blank with invalid saved falls back", "al ice", "\n", "anonymous"},
		{"blank with no saved", "", "\n", "anonymous"},
		{"typed name wins over saved", "alice", "bob\n", "bob"},
		{"invalid saved is never offered", "bad\u00e9name", "\n", "anonymous"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder

			got := promptNickname(bufio.NewReader(strings.NewReader(tc.input)), &out, tc.saved)

			if got != tc.want {
				t.Errorf("promptNickname(saved=%q) = %q, want %q", tc.saved, got, tc.want)
			}
		})
	}
}

func TestPromptNicknameEOFWithoutInput(t *testing.T) {
	var out strings.Builder

	got := promptNickname(bufio.NewReader(strings.NewReader("")), &out, "")

	if got != "anonymous" {
		t.Errorf("eof nick = %q, want anonymous", got)
	}
}

func TestCmdNickRejectsInvalid(t *testing.T) {
	m := testModel()
	m.messages = []chatLine{}

	handled, _ := handleCommand(&m, "/nick b\u00e4d")

	if !handled {
		t.Fatal("/nick not handled")
	}

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent %+v on invalid nick, want nothing", msgs)
	}

	if m.nick != "alice" {
		t.Errorf("m.nick = %q, want unchanged alice", m.nick)
	}

	found := false

	for _, line := range m.messages {
		if strings.Contains(line.rendered, "Invalid nickname") {
			found = true
		}
	}

	if !found {
		t.Error("no Invalid nickname feedback appended")
	}
}

func TestNicknameErrorReasons(t *testing.T) {
	cases := map[string]string{
		strings.Repeat("x", shared.MaxNicknameLength+1): "at most 32",
		"a b":      "spaces are not allowed",
		"h\u00e9y": "printable ASCII",
	}

	for nick, want := range cases {
		if got := nicknameError(nick); !strings.Contains(got, want) {
			t.Errorf("nicknameError(%q) = %q, want it to contain %q", nick, got, want)
		}
	}
}

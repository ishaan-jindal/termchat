package main

import (
	"strings"
	"testing"

	"termchat/shared"
)

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

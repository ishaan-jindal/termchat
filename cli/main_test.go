package main

import (
	"strings"
	"testing"
)

func TestParseHostModeWithRoomAndPort(t *testing.T) {
	opts, err := parseArgs([]string{"host", "FROG", "--port", "9000"})
	if err != nil {
		t.Fatal(err)
	}

	if !opts.HostMode {
		t.Fatal("expected host mode")
	}
	if opts.Room != "FROG" {
		t.Fatalf("room = %q, want FROG", opts.Room)
	}
	if opts.Port != 9000 {
		t.Fatalf("port = %d, want 9000", opts.Port)
	}
	if got := websocketURL(opts); got != "ws://localhost:9000/ws" {
		t.Fatalf("websocketURL = %q", got)
	}
}

func TestParseLANJoinFlagsAfterRoom(t *testing.T) {
	opts, err := parseArgs([]string{"FROG", "--host", "192.168.1.42", "--port", "9000"})
	if err != nil {
		t.Fatal(err)
	}

	if opts.HostMode {
		t.Fatal("did not expect host mode")
	}
	if opts.Room != "FROG" {
		t.Fatalf("room = %q, want FROG", opts.Room)
	}
	if got := websocketURL(opts); got != "ws://192.168.1.42:9000/ws" {
		t.Fatalf("websocketURL = %q", got)
	}
}

func TestExplicitServerTakesPriority(t *testing.T) {
	opts, err := parseArgs([]string{
		"FROG",
		"--host", "192.168.1.42",
		"--port", "9000",
		"--server", "ws://example.test/ws",
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := websocketURL(opts); got != "ws://example.test/ws" {
		t.Fatalf("websocketURL = %q", got)
	}
}

func TestParseHelp(t *testing.T) {
	opts, err := parseArgs([]string{"--help"})
	if err != nil {
		t.Fatal(err)
	}

	if !opts.Help {
		t.Fatal("expected help flag")
	}
}

func TestDiscoverBaseURL(t *testing.T) {
	opts, err := parseArgs([]string{"discover"})
	if err != nil {
		t.Fatal(err)
	}

	if got := discoverBaseURL(opts); got != "https://termchat.sacred99.online" {
		t.Fatalf("default base = %q", got)
	}

	opts, err = parseArgs([]string{"discover", "--server", "ws://example.test/ws"})
	if err != nil {
		t.Fatal(err)
	}

	if got := discoverBaseURL(opts); got != "http://example.test" {
		t.Fatalf("server-derived base = %q", got)
	}

	opts, err = parseArgs([]string{"discover", "--server", "wss://example.test/ws"})
	if err != nil {
		t.Fatal(err)
	}

	if got := discoverBaseURL(opts); got != "https://example.test" {
		t.Fatalf("wss-derived base = %q", got)
	}

	opts, err = parseArgs([]string{"discover", "--host", "192.168.1.42", "--port", "9000"})
	if err != nil {
		t.Fatal(err)
	}

	if got := discoverBaseURL(opts); got != "http://192.168.1.42:9000" {
		t.Fatalf("host-derived base = %q", got)
	}
}

func TestParseArgsEdges(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string // expected error substring, empty = no error
	}{
		{"room flag without value", []string{"--room"}, "requires a value"},
		{"server flag without value", []string{"--server"}, "requires a value"},
		{"host flag without value", []string{"--host"}, "requires a value"},
		{"password flag without value", []string{"--password"}, "requires a value"},
		{"theme flag without value", []string{"--theme"}, "requires a value"},
		{"unknown theme", []string{"--theme", "bogus"}, "unknown theme"},
		{"port out of range", []string{"--port", "99999"}, "invalid port"},
		{"port negative", []string{"--port", "-1"}, "invalid port"},
		{"port not a number", []string{"--port", "abc"}, "invalid port"},
		{"unknown flag", []string{"--bogus"}, "unknown flag"},
		{"two positionals", []string{"FROG", "FROG2"}, "unexpected argument"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseArgs(c.args)

			if c.want == "" {
				if err != nil {
					t.Fatalf("parseArgs(%v) = %v, want nil", c.args, err)
				}

				return
			}

			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("parseArgs(%v) = %v, want error containing %q", c.args, err, c.want)
			}
		})
	}
}

func TestParseInlineFlagValues(t *testing.T) {
	opts, err := parseArgs([]string{"--room=FROG", "--port=9000", "--password=secret", "--theme=light"})
	if err != nil {
		t.Fatal(err)
	}

	if opts.Room != "FROG" {
		t.Errorf("room = %q, want FROG", opts.Room)
	}

	if opts.Port != 9000 {
		t.Errorf("port = %d, want 9000", opts.Port)
	}

	if opts.Password != "secret" {
		t.Errorf("password = %q, want secret", opts.Password)
	}

	if opts.Theme != "light" {
		t.Errorf("theme = %q, want light", opts.Theme)
	}
}

func TestParseHostModeTakesPrecedenceForURL(t *testing.T) {
	opts, err := parseArgs([]string{"host", "FROG", "--host", "192.168.1.42"})
	if err != nil {
		t.Fatal(err)
	}

	if !opts.HostMode {
		t.Fatal("expected host mode")
	}

	// Host mode always connects to the local server.
	if got := websocketURL(opts); got != "ws://localhost:8080/ws" {
		t.Errorf("websocketURL = %q, want localhost", got)
	}
}

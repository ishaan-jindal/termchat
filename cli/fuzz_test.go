package main

import (
	"strings"
	"testing"
)

func FuzzParseArgs(f *testing.F) {
	f.Add("host")
	f.Add("discover")
	f.Add("FROG")
	f.Add("--room")
	f.Add("--port")
	f.Add("--server=wss://example.test/ws")
	f.Add("--password=secret")
	f.Add("--theme=dracula")
	f.Add("-h")

	f.Fuzz(func(t *testing.T, arg string) {
		args := strings.Fields(arg)

		// parseArgs must never panic, regardless of input.
		opts, err := parseArgs(args)

		if err != nil {
			return
		}

		_ = opts
	})
}

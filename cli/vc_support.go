package main

import "runtime"

// vcUnsupportedReason returns a user-facing message when goos cannot join
// the voice/video call, or "" when it can. VC is Linux-only for now.
func vcUnsupportedReason(goos string) string {
	if goos == "linux" {
		return ""
	}

	return "voice/video chat is linux-only for now"
}

// vcUnsupported is vcUnsupportedReason for the running platform.
func vcUnsupported() string {
	return vcUnsupportedReason(runtime.GOOS)
}

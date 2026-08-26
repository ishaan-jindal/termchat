//go:build windows

package main

import (
	"os/exec"
)

func micInputCandidates(device string) [][]string {
	if device == "" {
		return nil
	}

	return [][]string{{"-f", "dshow", "-i", "audio=" + device}}
}

func setPgid(_ *exec.Cmd) {}

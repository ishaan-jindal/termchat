//go:build linux

package main

import (
	"os/exec"
	"syscall"
)

func micInputCandidates(device string) [][]string {
	if device != "" {
		return [][]string{
			{"-f", "pulse", "-i", device},
			{"-f", "alsa", "-i", device},
		}
	}

	return [][]string{
		{"-f", "pulse", "-i", "default"},
		{"-f", "alsa", "-i", "default"},
	}
}

func setPgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

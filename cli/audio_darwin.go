//go:build darwin

package main

import (
	"os/exec"
	"syscall"
)

func micInputCandidates(device string) [][]string {
	name := ":default"

	if device != "" {
		name = ":" + device
	}

	return [][]string{{"-f", "avfoundation", "-i", name}}
}

func setPgid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

//go:build !linux && !darwin && !windows

package main

import (
	"os/exec"
)

func micInputCandidates(_ string) [][]string {
	return nil
}

func setPgid(_ *exec.Cmd) {}

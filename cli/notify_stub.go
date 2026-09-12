//go:build !linux && !darwin && !windows

package main

func notify(_, _ string) {
}

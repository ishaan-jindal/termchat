//go:build linux

package main

import (
	"fmt"

	"termchat/shared"
)

func platformCamInputs(device string) [][]string {
	if device == "" {
		device = "/dev/video0"
	}

	return [][]string{{
		"-f", "v4l2",
		"-framerate", fmt.Sprint(shared.VideoCapFPS),
		"-video_size", fmt.Sprintf("%dx%d", shared.VideoCapWidth, shared.VideoCapHeight),
		"-i", device,
	}}
}

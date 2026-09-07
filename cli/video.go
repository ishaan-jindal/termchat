package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"termchat/shared"
)

// camCapture is one running ffmpeg webcam process emitting an MJPEG stream.
type camCapture struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	exited   chan struct{}
	tail     *stderrTail
	stopOnce sync.Once
}

const (
	// camQuality is the ffmpeg MJPEG qscale: 2 is best, 31 is worst.
	camQuality = 8

	// camTrialWindow bounds how long a candidate input may take to start.
	camTrialWindow = 500 * time.Millisecond
)

// startCam launches ffmpeg with the first camera input that stays alive
// past the trial window. TERMCHAT_CAM_SOURCE overrides the input with a
// file (e.g. a JPEG still) for headless testing.
func startCam(device string) (*camCapture, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is required for video; install it and retry")
	}

	var lastErr error

	for _, input := range camInputCandidates(device) {
		cc, candErr := tryCamCandidate(bin, input)

		if candErr == nil {
			return cc, nil
		}

		lastErr = candErr
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.New("no usable camera input found")
}

// camInputCandidates returns the ffmpeg inputs to try, honoring the test
// override before the platform candidates.
func camInputCandidates(device string) [][]string {
	if src := os.Getenv("TERMCHAT_CAM_SOURCE"); src != "" {
		return [][]string{{"-loop", "1", "-framerate", fmt.Sprint(shared.VideoCapFPS), "-i", src}}
	}

	return platformCamInputs(device)
}

func tryCamCandidate(bin string, input []string) (*camCapture, error) {
	args := []string{"-nostdin", "-loglevel", "warning"}
	args = append(args, input...)
	args = append(args,
		"-vf", fmt.Sprintf("scale=%d:%d", shared.VideoCapWidth, shared.VideoCapHeight),
		"-q:v", fmt.Sprint(camQuality),
		"-f", "mjpeg", "pipe:1",
	)

	cmd := exec.Command(bin, args...)

	setPgid(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("capture pipe: %w", err)
	}

	tail := newStderrTail(2048)
	cmd.Stderr = tail

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("starting capture %q: %w", input[len(input)-1], err)
	}

	cc := &camCapture{
		cmd:    cmd,
		stdout: stdout,
		exited: make(chan struct{}),
		tail:   tail,
	}

	go func() {
		cmd.Wait()
		close(cc.exited)
	}()

	select {
	case <-cc.exited:
		msg := tail.String()

		if msg == "" {
			msg = "exited immediately"
		}

		return nil, fmt.Errorf("capture input %q failed: %s", input[len(input)-1], msg)
	case <-time.After(camTrialWindow):
	}

	return cc, nil
}

// stop kills the process and waits for its exit.
func (cc *camCapture) stop() {
	cc.stopOnce.Do(func() {
		if cc.cmd.Process != nil {
			cc.cmd.Process.Kill()
		}

		<-cc.exited
	})
}

// nextJPEG reads one baseline JPEG frame from the pipe. Stuffed 0xFF bytes
// in entropy data are always followed by 0x00, so SOI/EOI scans are exact.
func nextJPEG(br *bufio.Reader) ([]byte, error) {
	for {
		b, err := br.ReadByte()
		if err != nil {
			return nil, err
		}

		if b != 0xFF {
			continue
		}

		m, err := br.ReadByte()
		if err != nil {
			return nil, err
		}

		if m == 0xD8 {
			return readUntilEOI(br)
		}
	}
}

func readUntilEOI(br *bufio.Reader) ([]byte, error) {
	frame := []byte{0xFF, 0xD8}

	for {
		if len(frame) > shared.VideoMaxFrameBytes {
			return nil, errors.New("video frame exceeded the size cap")
		}

		b, err := br.ReadByte()
		if err != nil {
			return nil, err
		}

		frame = append(frame, b)

		if b != 0xFF {
			continue
		}

		m, err := br.ReadByte()
		if err != nil {
			return nil, err
		}

		frame = append(frame, m)

		if m == 0xD9 {
			return frame, nil
		}
	}
}

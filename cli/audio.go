package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"termchat/shared"
)

const (
	ringCapacity   = 8
	ringStaleAfter = 2 * time.Second
)

var chunkDuration = shared.AudioChunkSamples * time.Second / shared.AudioSampleRate

// chunkRing buffers one speaker's audio chunks in arrival order.
type chunkRing struct {
	chunks   [][]int16
	lastSeen time.Time
}

func (r *chunkRing) push(samples []int16) {
	if len(r.chunks) >= ringCapacity {
		r.chunks = r.chunks[1:]
	}

	r.chunks = append(r.chunks, samples)
	r.lastSeen = time.Now()
}

func (r *chunkRing) pop() ([]int16, bool) {
	if len(r.chunks) == 0 {
		return nil, false
	}

	head := r.chunks[0]
	r.chunks = r.chunks[1:]

	return head, true
}

func (r *chunkRing) stale(now time.Time) bool {
	return now.Sub(r.lastSeen) > ringStaleAfter
}

// mixInto adds src into dst with saturation so overlapping speakers clip
// instead of wrapping.
func mixInto(dst, src []int16) {
	for i := range dst {
		sum := int32(dst[i]) + int32(src[i])

		if sum > 32767 {
			sum = 32767
		}

		if sum < -32768 {
			sum = -32768
		}

		dst[i] = int16(sum)
	}
}

func bytesToSamples(b []byte) []int16 {
	out := make([]int16, len(b)/2)

	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}

	return out
}

func samplesToBytes(s []int16) []byte {
	out := make([]byte, len(s)*2)

	for i, v := range s {
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}

	return out
}

// micCapture is one running ffmpeg microphone process.
type micCapture struct {
	cmd      *exec.Cmd
	stdout   io.ReadCloser
	exited   chan struct{}
	stopOnce sync.Once
}

// startMic launches ffmpeg with the first input candidate that stays alive
// past a short trial window.
func startMic(device string) (*micCapture, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is required for voice; install it and retry")
	}

	for _, input := range micInputCandidates(device) {
		mc := tryMicCandidate(bin, input)

		if mc != nil {
			return mc, nil
		}
	}

	return nil, errors.New("no usable microphone input found")
}

func tryMicCandidate(bin string, input []string) *micCapture {
	args := []string{"-nostdin", "-loglevel", "quiet"}
	args = append(args, input...)
	args = append(args,
		"-fflags", "+nobuffer",
		"-flags", "low_delay",
		"-ar", fmt.Sprint(shared.AudioSampleRate),
		"-ac", fmt.Sprint(shared.AudioChannels),
		"-f", "s16le", "pipe:1",
	)

	cmd := exec.Command(bin, args...)
	cmd.Stderr = io.Discard

	setPgid(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil
	}

	err = cmd.Start()
	if err != nil {
		return nil
	}

	mc := &micCapture{
		cmd:    cmd,
		stdout: stdout,
		exited: make(chan struct{}),
	}

	go func() {
		cmd.Wait()
		close(mc.exited)
	}()

	select {
	case <-mc.exited:
		return nil
	case <-time.After(250 * time.Millisecond):
	}

	return mc
}

// stop kills the process and waits for its exit.
func (mc *micCapture) stop() {
	mc.stopOnce.Do(func() {
		if mc.cmd.Process != nil {
			mc.cmd.Process.Kill()
		}

		<-mc.exited
	})
}

// VoiceSession bundles the media connection with its audio processes. All
// methods run on the Bubble Tea update goroutine.
type VoiceSession struct {
	conn     *MediaConn
	tx       bool
	mic      *micCapture
	playCmd  *exec.Cmd
	playPipe io.WriteCloser
}

func (s *VoiceSession) startPlayout() error {
	pc, err := playerCommand()
	if err != nil {
		return err
	}

	stdin, err := pc.StdinPipe()
	if err != nil {
		return err
	}

	pc.Stderr = io.Discard

	err = pc.Start()
	if err != nil {
		stdin.Close()

		return fmt.Errorf("starting ffplay: %w", err)
	}

	s.playCmd = pc
	s.playPipe = stdin

	go s.playoutLoop(stdin)

	return nil
}

func (s *VoiceSession) Shutdown() {
	s.stopTx()
	s.conn.close()

	if s.playPipe != nil {
		s.playPipe.Close()
	}

	if s.playCmd != nil {
		s.playCmd.Wait()
		s.playCmd = nil
	}
}

func (s *VoiceSession) stopTx() {
	if s.mic == nil {
		s.tx = false

		return
	}

	s.tx = false
	s.mic.stop()
	s.mic = nil
}

// playoutLoop consumes inbound frames into per-speaker rings and writes one
// mixed 40 ms chunk per tick; it is the only consumer of conn.inbox.
func (s *VoiceSession) playoutLoop(stdin io.WriteCloser) {
	ticker := time.NewTicker(chunkDuration)
	defer ticker.Stop()

	rings := map[uint32]*chunkRing{}
	mix := make([]int16, shared.AudioChunkSamples)
	started := false

	for {
		select {
		case frame := <-s.conn.inbox:
			kind, codec, id, payload, ok := shared.ParseMediaFrame(frame)

			if !ok || kind != shared.MediaKindAudio || codec != shared.MediaCodecPCM16 {
				continue
			}

			if id == 0 || len(payload)%2 != 0 {
				continue
			}

			r := rings[id]

			if r == nil {
				r = &chunkRing{}
				rings[id] = r
			}

			r.push(bytesToSamples(payload))
			started = true

		case now := <-ticker.C:
			for i := range mix {
				mix[i] = 0
			}

			active := false

			for id, r := range rings {
				if r.stale(now) {
					delete(rings, id)

					continue
				}

				chunk, ok := r.pop()

				if ok {
					mixInto(mix, chunk)
					active = true
				}
			}

			if !active && !started {
				continue
			}

			if _, err := stdin.Write(samplesToBytes(mix)); err != nil {
				return
			}

		case <-s.conn.done:
			return
		}
	}
}

// pumpCapture forwards mic chunks to the media connection until the process
// exits.
func pumpCapture(s *VoiceSession, mic *micCapture) {
	buf := make([]byte, shared.AudioChunkBytes)

	for {
		_, err := io.ReadFull(mic.stdout, buf)
		if err != nil {
			return
		}

		frame := shared.EncodeAudioFrame(shared.MediaKindAudio, shared.MediaCodecPCM16, 0, buf)
		s.conn.trySend(frame)
	}
}

type voiceMicStoppedMsg struct{ mic *micCapture }

func waitForMicStop(mic *micCapture) tea.Cmd {
	return func() tea.Msg {
		<-mic.exited

		return voiceMicStoppedMsg{mic: mic}
	}
}

// toggleTalk arms or disarms the microphone; it returns a cmd that reports
// unexpected capture deaths to the TUI.
func toggleTalk(m *Model) tea.Cmd {
	if m.voice == nil {
		appendUI(m, "join voice first with /voice on")

		return nil
	}

	if m.voice.tx {
		m.voice.stopTx()
		appendUI(m, "microphone muted")

		return nil
	}

	mic, err := startMic(m.VoiceDevice)
	if err != nil {
		appendUI(m, "voice transmit failed: "+err.Error())

		return nil
	}

	m.voice.mic = mic
	m.voice.tx = true
	appendUI(m, "transmitting - ctrl+t to mute")

	go pumpCapture(m.voice, mic)

	return waitForMicStop(mic)
}

// playerCommand assembles the ffplay raw PCM playback command.
func playerCommand() (*exec.Cmd, error) {
	bin, err := exec.LookPath("ffplay")
	if err != nil {
		return nil, errors.New("ffplay is required to hear voice; install ffmpeg and retry")
	}

	args := []string{
		"-nodisp",
		"-autoexit",
		"-loglevel", "quiet",
		"-fflags", "+nobuffer",
		"-infbuf",
		"-f", "s16le",
		"-ar", fmt.Sprint(shared.AudioSampleRate),
		"-ac", fmt.Sprint(shared.AudioChannels),
		"-i", "pipe:0",
	}

	return exec.Command(bin, args...), nil
}

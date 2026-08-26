package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
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
	tail     *stderrTail
	stopOnce sync.Once
}

// stderrTail is an io.Writer keeping the last max bytes of a subprocess's
// stderr so failures can be reported verbatim.
type stderrTail struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newStderrTail(max int) *stderrTail {
	return &stderrTail{max: max}
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buf = append(s.buf, p...)

	if over := len(s.buf) - s.max; over > 0 {
		s.buf = s.buf[over:]
	}

	return len(p), nil
}

func (s *stderrTail) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return string(s.buf)
}

// playoutProc is one supervised player process.
type playoutProc struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	exited   chan struct{}
	tail     *stderrTail
	stopOnce sync.Once
	header   []byte // optional stream prefix; nil for raw PCM players
	name     string
}

// wavHeader returns a canonical 44-byte PCM WAV header describing the
// shared voice format; the size fields are streaming placeholders.
func wavHeader() []byte {
	buf := make([]byte, 44)

	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], 0x7FFFFFF6)
	copy(buf[8:12], "WAVE")

	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1)
	binary.LittleEndian.PutUint16(buf[22:], uint16(shared.AudioChannels))
	binary.LittleEndian.PutUint32(buf[24:], shared.AudioSampleRate)
	byteRate := shared.AudioSampleRate * uint32(shared.AudioChannels) * 2
	binary.LittleEndian.PutUint32(buf[28:], byteRate)
	binary.LittleEndian.PutUint16(buf[32:], uint16(shared.AudioChannels*2))
	binary.LittleEndian.PutUint16(buf[34:], 16)

	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:], 0x7FFFFFD2)

	return buf
}

// playerKind identifies the resolved audio playback backend.
type playerKind int

const (
	playerPaplay playerKind = iota
	playerFfplay
)

// resolvePlayer picks the playback backend: paplay first on Linux, ffplay
// everywhere else; TERMCHAT_VOICE_PLAYER forces one for testing.
func resolvePlayer() (playerKind, error) {
	switch os.Getenv("TERMCHAT_VOICE_PLAYER") {
	case "paplay":
		return playerPaplay, nil
	case "ffplay":
		return playerFfplay, nil
	case "":
	default:
		return 0, fmt.Errorf("unknown TERMCHAT_VOICE_PLAYER %q (use paplay or ffplay)", os.Getenv("TERMCHAT_VOICE_PLAYER"))
	}

	if runtime.GOOS == "linux" {
		if _, err := exec.LookPath("paplay"); err == nil {
			return playerPaplay, nil
		}
	}

	return playerFfplay, nil
}

// playerSpec returns the player's binary name, args, and optional stream
// header for the chosen kind; a nil header means the player consumes raw
// PCM. It is pure and safe to assert on in any environment.
func playerSpec(kind playerKind) (string, []string, []byte) {
	switch kind {
	case playerPaplay:
		args := []string{
			"--raw",
			"--format=s16le",
			"--rate", fmt.Sprint(shared.AudioSampleRate),
			"--channels", fmt.Sprint(shared.AudioChannels),
		}

		return "paplay", args, nil

	case playerFfplay:
		args := []string{
			"-nodisp",
			"-autoexit",
			"-loglevel", "warning",
			"-infbuf",
			"-i", "pipe:0",
		}

		return "ffplay", args, wavHeader()
	}

	return "", nil, nil
}

// playerCommand resolves the spec and verifies the binary exists on PATH,
// preserving the user-facing guidance for missing players.
func playerCommand(kind playerKind) (string, []string, []byte, error) {
	bin, args, header := playerSpec(kind)

	if bin == "" {
		return "", nil, nil, errors.New("unknown player kind")
	}

	if _, err := exec.LookPath(bin); err != nil {
		switch kind {
		case playerPaplay:
			return "", nil, nil, errors.New("paplay not found")
		case playerFfplay:
			return "", nil, nil, errors.New("ffplay is required to hear voice; install ffmpeg and retry")
		}
	}

	return bin, args, header, nil
}

// startPlayProc launches the resolved player and fails fast when it dies
// during the trial window; its stderr tail rides along in the error.
func startPlayProc(kind playerKind) (*playoutProc, error) {
	bin, args, header, err := playerCommand(kind)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(bin, args...)

	setPgid(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("player stdin: %w", err)
	}

	tail := newStderrTail(2048)
	cmd.Stderr = tail

	err = cmd.Start()
	if err != nil {
		stdin.Close()

		return nil, fmt.Errorf("starting player: %w", err)
	}

	pp := &playoutProc{
		cmd:    cmd,
		stdin:  stdin,
		exited: make(chan struct{}),
		tail:   tail,
		header: header,
		name:   filepath.Base(bin),
	}

	go func() {
		cmd.Wait()
		close(pp.exited)
	}()

	select {
	case <-pp.exited:
		stdin.Close()

		msg := tail.String()
		if msg == "" {
			msg = "exited immediately"
		}

		return nil, fmt.Errorf("%s failed to start: %s", pp.name, msg)
	case <-time.After(400 * time.Millisecond):
	}

	return pp, nil
}

func (pp *playoutProc) stop() {
	pp.stopOnce.Do(func() {
		pp.stdin.Close()

		select {
		case <-pp.exited:
		default:
			if pp.cmd.Process != nil {
				pp.cmd.Process.Kill()
			}

			<-pp.exited
		}
	})
}

// startMic launches ffmpeg with the first input candidate that stays alive
// past a short trial window.
func startMic(device string) (*micCapture, error) {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("ffmpeg is required for voice; install it and retry")
	}

	var lastErr error

	for _, input := range micInputCandidates(device) {
		mc, candErr := tryMicCandidate(bin, input)

		if candErr == nil {
			return mc, nil
		}

		lastErr = candErr
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.New("no usable microphone input found")
}

func tryMicCandidate(bin string, input []string) (*micCapture, error) {
	args := []string{"-nostdin", "-loglevel", "warning"}
	args = append(args, input...)
	args = append(args,
		"-fflags", "+nobuffer",
		"-flags", "low_delay",
		"-ar", fmt.Sprint(shared.AudioSampleRate),
		"-ac", fmt.Sprint(shared.AudioChannels),
		"-f", "s16le", "pipe:1",
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

	mc := &micCapture{
		cmd:    cmd,
		stdout: stdout,
		exited: make(chan struct{}),
		tail:   tail,
	}

	go func() {
		cmd.Wait()
		close(mc.exited)
	}()

	select {
	case <-mc.exited:
		msg := tail.String()

		if msg == "" {
			msg = "exited immediately"
		}

		return nil, fmt.Errorf("capture input %q failed: %s", input[len(input)-1], msg)
	case <-time.After(250 * time.Millisecond):
	}

	return mc, nil
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
	conn *MediaConn
	tx   bool
	mic  *micCapture
	play *playoutProc

	txSince    time.Time
	playerName string

	sentFrames atomic.Uint64
	recvFrames atomic.Uint64

	lastSent atomic.Int64 // unix millis of the last voiced outbound chunk
	lastRecv atomic.Int64 // unix millis of the last voiced inbound chunk

	dumps *voiceDumps
}

// voicePeakThreshold is the minimum sample magnitude treated as speech for
// the TX/RX activity lights; open-mic room tone stays below it.
const voicePeakThreshold = 700

// chunkPeak returns the loudest sample magnitude in a chunk.
func chunkPeak(samples []int16) int16 {
	var peak int32

	for _, v := range samples {
		a := int32(v)

		if a < 0 {
			a = -a
		}

		if a > peak {
			peak = a
		}
	}

	if peak > 32767 {
		peak = 32767
	}

	return int16(peak)
}

// voiceDumps mirrors both pipeline ends into WAV files when
// TERMCHAT_VOICE_DEBUG points at a directory.
type voiceDumps struct {
	tx *os.File
	rx *os.File
}

func openVoiceDumps() (*voiceDumps, error) {
	dir := os.Getenv("TERMCHAT_VOICE_DEBUG")
	if dir == "" {
		return nil, nil
	}

	pid := os.Getpid()

	tx, err := os.Create(filepath.Join(dir, fmt.Sprintf("tx-%d.wav", pid)))
	if err != nil {
		return nil, err
	}

	rx, err := os.Create(filepath.Join(dir, fmt.Sprintf("rx-%d.wav", pid)))
	if err != nil {
		tx.Close()

		return nil, err
	}

	header := wavHeader()
	tx.Write(header)
	rx.Write(header)

	return &voiceDumps{tx: tx, rx: rx}, nil
}

func (d *voiceDumps) writeTX(pcm []byte) {
	if d == nil {
		return
	}

	d.tx.Write(pcm)
}

func (d *voiceDumps) writeRX(mixed []byte) {
	if d == nil {
		return
	}

	d.rx.Write(mixed)
}

func (d *voiceDumps) close() {
	if d == nil {
		return
	}

	d.tx.Close()
	d.rx.Close()
}

func (s *VoiceSession) startPlayout() error {
	kind, err := resolvePlayer()
	if err != nil {
		return err
	}

	pp, err := startPlayProc(kind)
	if err != nil {
		return err
	}

	s.playerName = pp.name

	if len(pp.header) > 0 {
		_, err = pp.stdin.Write(pp.header)
		if err != nil {
			pp.stop()

			return fmt.Errorf("writing wav header: %w", err)
		}
	}

	s.play = pp

	go s.playoutLoop(pp.stdin)

	return nil
}

// playerStatus describes the playback backend for /voice diagnostics.
func (s *VoiceSession) playerStatus() string {
	if s.play == nil {
		return "player: none"
	}

	state := "alive"

	select {
	case <-s.play.exited:
		state = "dead"
	default:
	}

	status := fmt.Sprintf("player %s %s", s.play.name, state)

	tail := s.play.tail.String()

	if tail != "" {
		if len(tail) > 160 {
			tail = "..." + tail[len(tail)-160:]
		}

		status += " - tail: " + tail
	}

	return status
}

func (s *VoiceSession) Shutdown() {
	s.stopTx()
	s.conn.close()

	if s.play != nil {
		s.play.stop()
		s.play = nil
	}

	s.dumps.close()
	s.dumps = nil
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
			s.recvFrames.Add(1)
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

			out := samplesToBytes(mix)
			s.dumps.writeRX(out)

			if chunkPeak(mix) >= voicePeakThreshold {
				s.lastRecv.Store(time.Now().UnixMilli())
			}

			if _, err := stdin.Write(out); err != nil {
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
		s.sentFrames.Add(1)
		s.dumps.writeTX(buf)

		if chunkPeak(bytesToSamples(buf)) >= voicePeakThreshold {
			s.lastSent.Store(time.Now().UnixMilli())
		}
	}
}

type voiceMicStoppedMsg struct {
	mic  *micCapture
	tail string
}

type voicePlaybackStoppedMsg struct {
	play *playoutProc
	tail string
}

func waitForMicStop(mic *micCapture) tea.Cmd {
	return func() tea.Msg {
		<-mic.exited

		return voiceMicStoppedMsg{mic: mic, tail: mic.tail.String()}
	}
}

func waitForPlaybackStop(play *playoutProc) tea.Cmd {
	return func() tea.Msg {
		<-play.exited

		return voicePlaybackStoppedMsg{play: play, tail: play.tail.String()}
	}
}

type voiceActivityTickMsg struct{}

// voiceActivityTicker drives the footer TX/RX activity lights while a
// session is joined; Update re-arms it until the session ends.
func voiceActivityTicker() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return voiceActivityTickMsg{}
	})
}

// toggleTalk arms or disarms the microphone; it returns a cmd that reports
// unexpected capture deaths to the TUI.
func toggleTalk(m *Model) tea.Cmd {
	if m.voice == nil {
		appendUI(m, "join voice first with /voice on")

		return nil
	}

	if m.voice.tx {
		if time.Since(m.voice.txSince) > time.Second && m.voice.sentFrames.Load() == 0 {
			appendUI(m, "muted - no audio was captured; check 'pactl list short sources' or set voice_device in ~/.termchat/config.json")
		} else {
			appendUI(m, "microphone muted")
		}

		m.voice.stopTx()

		return nil
	}

	mic, err := startMic(m.VoiceDevice)
	if err != nil {
		appendUI(m, "voice transmit failed: "+err.Error())

		return nil
	}

	m.voice.mic = mic
	m.voice.tx = true
	m.voice.txSince = time.Now()
	appendUI(m, "transmitting - ctrl+t to mute")

	go pumpCapture(m.voice, mic)

	return waitForMicStop(mic)
}

// VoiceSession bundles

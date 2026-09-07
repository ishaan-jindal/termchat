package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"termchat/shared"
)

// videoStaleAfter drops a peer tile whose last frame is older than this.
const videoStaleAfter = 3 * time.Second

// maxVideoPeers caps stored streams; the oldest is evicted past the cap.
const maxVideoPeers = 8

// videoPeerFrame is one stream's latest decoded frame.
type videoPeerFrame struct {
	pix     []byte
	w       int
	h       int
	updated time.Time
}

// VideoSession bundles a video session atop the shared media connection.
// The Model owns the conn lifecycle; Shutdown only stops local processes.
type VideoSession struct {
	conn *MediaConn
	tx   bool
	cam  *camCapture

	done     chan struct{}
	stopOnce sync.Once
	txSince  time.Time

	sentFrames atomic.Uint64
	recvFrames atomic.Uint64

	lastSent atomic.Int64 // unix millis of the last outbound frame
	lastRecv atomic.Int64 // unix millis of the last inbound frame

	mu    sync.Mutex
	peers map[uint32]*videoPeerFrame
	self  *videoPeerFrame // looped-back local preview while transmitting
}

// startVideoSession attaches a video session to the shared media conn and
// starts the camera; a dead camera still leaves watch mode running.
func (m *Model) startVideoSession() tea.Cmd {
	vs := &VideoSession{
		conn:  m.media,
		done:  make(chan struct{}),
		peers: map[uint32]*videoPeerFrame{},
	}
	m.video = vs
	refitLayout(m)

	go vs.recvLoop()

	cam, err := startCam(m.CameraDevice)
	if err != nil {
		appendUI(m, "video joined (watching) - camera unavailable: "+err.Error())

		return videoTicker()
	}

	startCameraAndPump(vs, cam)
	appendUI(m, "video on - ctrl+v toggles camera")

	return tea.Batch(waitForCamStop(cam), videoTicker())
}

// startCameraAndPump arms the camera and launches the pump that reads its
// frames, forwards them to peers, and feeds the local preview.
func startCameraAndPump(vs *VideoSession, cam *camCapture) {
	vs.cam = cam
	vs.tx = true
	vs.txSince = time.Now()

	go pumpCamera(vs, cam)
}

func (s *VideoSession) stop() {
	s.stopOnce.Do(func() {
		close(s.done)
	})
}

// Shutdown stops capture and the receive loop and clears stored frames.
func (s *VideoSession) Shutdown() {
	s.stop()
	s.stopTx()

	s.mu.Lock()
	s.peers = map[uint32]*videoPeerFrame{}
	s.self = nil
	s.mu.Unlock()
}

func (s *VideoSession) stopTx() {
	if s.cam == nil {
		s.tx = false

		return
	}

	s.tx = false
	s.cam.stop()
	s.cam = nil

	s.mu.Lock()
	s.self = nil
	s.mu.Unlock()
}

// recvLoop decodes inbound video frames into per-sender latest frames.
func (s *VideoSession) recvLoop() {
	for {
		select {
		case frame := <-s.conn.video:
			kind, codec, id, payload, ok := shared.ParseMediaFrame(frame)

			if !ok || kind != shared.MediaKindVideo || codec != shared.MediaCodecJPEG {
				continue
			}

			if id == 0 || len(payload) == 0 || len(payload) > shared.VideoMaxFrameBytes {
				continue
			}

			pix, w, h, err := decodeVideoFrame(payload)
			if err != nil {
				continue
			}

			s.mu.Lock()

			if len(s.peers) >= maxVideoPeers {
				s.evictOldest()
			}

			s.peers[id] = &videoPeerFrame{pix: pix, w: w, h: h, updated: time.Now()}
			s.mu.Unlock()

			s.recvFrames.Add(1)
			s.lastRecv.Store(time.Now().UnixMilli())

		case <-s.done:
			return

		case <-s.conn.done:
			return
		}
	}
}

func (s *VideoSession) evictOldest() {
	var oldest uint32
	var oldestTime time.Time
	first := true

	for id, f := range s.peers {
		if first || f.updated.Before(oldestTime) {
			oldest, oldestTime, first = id, f.updated, false
		}
	}

	if !first {
		delete(s.peers, oldest)
	}
}

// prune drops stale peer frames; the TUI tick calls it on its goroutine.
func (s *VideoSession) prune() {
	cutoff := time.Now().Add(-videoStaleAfter)

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, f := range s.peers {
		if f.updated.Before(cutoff) {
			delete(s.peers, id)
		}
	}
}

// pumpCamera pixelates captured frames to the anonymity floor, encodes and
// forwards them to the media connection, and loops them back as the local
// preview until the process exits. Full-detail pixels never leave the sender.
func pumpCamera(s *VideoSession, cam *camCapture) {
	br := bufio.NewReader(cam.stdout)

	for {
		raw, err := nextJPEG(br)
		if err != nil {
			return
		}

		if len(raw) > shared.VideoMaxFrameBytes {
			continue
		}

		pix, w, h, err := decodeVideoFrame(raw)
		if err != nil {
			continue
		}

		pix = pixelateBuffer(pix, w, h, shared.VideoPixelDepth)

		jpeg, err := encodeVideoFrame(pix, w, h)
		if err != nil {
			continue
		}

		if len(jpeg) > shared.VideoMaxFrameBytes {
			continue
		}

		frame := shared.EncodeMediaFrame(shared.MediaKindVideo, shared.MediaCodecJPEG, 0, jpeg)
		s.conn.trySend(frame)
		s.sentFrames.Add(1)
		s.lastSent.Store(time.Now().UnixMilli())

		s.mu.Lock()
		s.self = &videoPeerFrame{pix: pix, w: w, h: h, updated: time.Now()}
		s.mu.Unlock()
	}
}

type videoCamStoppedMsg struct {
	cam  *camCapture
	tail string
}

func waitForCamStop(cam *camCapture) tea.Cmd {
	return func() tea.Msg {
		<-cam.exited

		return videoCamStoppedMsg{cam: cam, tail: cam.tail.String()}
	}
}

type videoTickMsg struct{}

// videoTicker repaints the video panel while a session is joined; Update
// re-arms it until the session ends.
func videoTicker() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return videoTickMsg{}
	})
}

// startVoiceSession attaches a voice session to the shared media conn.
func (m *Model) startVoiceSession() tea.Cmd {
	vs := &VoiceSession{conn: m.media}

	err := vs.startPlayout()
	if err != nil {
		appendUI(m, "voice unavailable: "+err.Error())

		return nil
	}

	dumps, dumpErr := openVoiceDumps()
	vs.dumps = dumps

	if dumpErr != nil {
		appendUI(m, "voice debug dumps unavailable: "+dumpErr.Error())
	} else if dumps != nil {
		pid := os.Getpid()
		dir := os.Getenv("TERMCHAT_VOICE_DEBUG")
		appendUI(m, fmt.Sprintf("voice debug: %s/tx-%d.wav and rx-%d.wav", dir, pid, pid))
	}

	m.voice = vs
	appendUI(m, "voice session joined - ctrl+t toggles talk")

	return tea.Batch(
		waitForPlaybackStop(vs.play),
		voiceActivityTicker(),
	)
}

// closeMediaIfIdle tears the shared media conn down once neither session
// uses it; it is a no-op while voice or video is attached.
func (m *Model) closeMediaIfIdle() {
	if m.voice != nil || m.video != nil {
		return
	}

	if m.media == nil {
		return
	}

	m.media.close()
	m.media = nil
}

// toggleVideo joins the video session or toggles the camera within it.
func toggleVideo(m *Model) tea.Cmd {
	if m.video == nil {
		m.wantVideo = true

		if m.media == nil {
			appendUI(m, "requesting video session...")
			trySend(m, Message{Type: "media_token"})
			m.tokenPending = true
			m.pendingCmd = mediaTimeoutCmd()

			return nil
		}

		return m.startVideoSession()
	}

	if m.video.tx {
		if time.Since(m.video.txSince) > time.Second && m.video.sentFrames.Load() == 0 {
			appendUI(m, "camera off - no frames were captured; check the camera or set cam_device in ~/.termchat/config.json")
		} else {
			appendUI(m, "camera off")
		}

		m.video.stopTx()

		return nil
	}

	cam, err := startCam(m.CameraDevice)
	if err != nil {
		appendUI(m, "camera failed: "+err.Error())

		return nil
	}

	startCameraAndPump(m.video, cam)
	appendUI(m, "camera on - ctrl+v to stop")

	return waitForCamStop(cam)
}

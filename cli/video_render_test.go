package main

import (
	"bufio"
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func solidNRGBA(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}

	return img
}

func encodeTestJPEG(t *testing.T, img image.Image) []byte {
	t.Helper()

	var buf bytes.Buffer

	err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
	if err != nil {
		t.Fatalf("encode test JPEG: %v", err)
	}

	return buf.Bytes()
}

func TestDecodeVideoFrameRoundtrip(t *testing.T) {
	raw := encodeTestJPEG(t, solidNRGBA(32, 24, color.NRGBA{R: 200, G: 30, B: 30, A: 255}))

	pix, w, h, err := decodeVideoFrame(raw)
	if err != nil {
		t.Fatalf("decodeVideoFrame: %v", err)
	}

	if w != 32 || h != 24 {
		t.Fatalf("size = %dx%d, want 32x24", w, h)
	}

	if len(pix) != w*h*3 {
		t.Fatalf("len(pix) = %d, want %d", len(pix), w*h*3)
	}

	if pix[0] < 150 || pix[1] > 90 || pix[2] > 90 {
		t.Errorf("first pixel = %v, want mostly red", pix[:3])
	}
}

func TestDecodeVideoFrameRejectsGarbage(t *testing.T) {
	_, _, _, err := decodeVideoFrame([]byte{1, 2, 3, 4})
	if err == nil {
		t.Error("decodeVideoFrame accepted garbage")
	}
}

func solidRGB(w, h int, r, g, b byte) []byte {
	pix := make([]byte, w*h*3)

	for i := 0; i < len(pix); i += 3 {
		pix[i], pix[i+1], pix[i+2] = r, g, b
	}

	return pix
}

func TestRenderVideoFrameColor(t *testing.T) {
	lines := renderVideoFrame(solidRGB(4, 4, 255, 0, 0), 4, 4, 2, 2, VideoModeColor)

	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}

	for _, line := range lines {
		if !strings.Contains(line, "38;2;255;0;0") {
			t.Errorf("line missing red fg: %q", line)
		}

		if strings.Count(line, "▀") != 2 {
			t.Errorf("line has %d cells, want 2: %q", strings.Count(line, "▀"), line)
		}

		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Errorf("line missing trailing reset: %q", line)
		}
	}
}

func TestRenderVideoFrameASCII(t *testing.T) {
	lines := renderVideoFrame(solidRGB(4, 4, 255, 255, 255), 4, 4, 3, 2, VideoModeASCII)

	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}

	for _, line := range lines {
		stripped := strings.TrimPrefix(line, "\x1b[0m")

		if stripped != "@@@" {
			t.Errorf("white ASCII line = %q, want %q", stripped, "@@@")
		}
	}
}

func TestRenderVideoFrameBadInput(t *testing.T) {
	if renderVideoFrame(nil, 4, 4, 2, 2, VideoModeColor) != nil {
		t.Error("nil pixels rendered")
	}

	if renderVideoFrame(solidRGB(2, 2, 1, 2, 3), 2, 2, 0, 2, VideoModeColor) != nil {
		t.Error("zero cols rendered")
	}

	if renderVideoFrame(solidRGB(2, 2, 1, 2, 3), 2, 2, 2, 0, VideoModeColor) != nil {
		t.Error("zero rows rendered")
	}
}

func TestLayoutVideoGrid(t *testing.T) {
	cases := []struct {
		n    int
		cols int
		rows int
	}{
		{1, 1, 1},
		{2, 2, 1},
		{3, 2, 2},
		{4, 2, 2},
		{9, 2, 2},
	}

	for _, c := range cases {
		cols, rows := layoutVideoGrid(c.n)

		if cols != c.cols || rows != c.rows {
			t.Errorf("layoutVideoGrid(%d) = %dx%d, want %dx%d", c.n, cols, rows, c.cols, c.rows)
		}
	}
}

func TestRenderVideoTilesCapsStreams(t *testing.T) {
	var tiles []videoTile

	for _, nick := range []string{"a", "b", "c", "d", "e"} {
		tiles = append(tiles, videoTile{nick: nick, pix: solidRGB(4, 4, 9, 9, 9), w: 4, h: 4})
	}

	lines := renderVideoTiles(tiles, 40, 20, VideoModeColor, 0)
	joined := strings.Join(lines, "\n")

	for _, nick := range []string{"a", "b", "c", "d"} {
		if !strings.Contains(joined, nick) {
			t.Errorf("tile %q missing from grid", nick)
		}
	}

	if strings.Contains(joined, " e ") || strings.Contains(joined, "\n e\n") {
		t.Error("fifth tile leaked past the cap")
	}

	if len(lines) != 20 {
		t.Errorf("len(lines) = %d, want 20", len(lines))
	}
}

func TestRenderVideoTilesEmpty(t *testing.T) {
	lines := renderVideoTiles(nil, 10, 4, VideoModeColor, 0)

	if len(lines) != 4 {
		t.Fatalf("len(lines) = %d, want 4", len(lines))
	}

	if strings.TrimSpace(strings.Join(lines, "")) != "" {
		t.Error("empty grid is not blank")
	}
}

func TestNextJPEGSkipsGarbage(t *testing.T) {
	first := append([]byte{0xFF, 0xD8, 0x11, 0xFF, 0x00, 0x22}, 0xFF, 0xD9)
	second := []byte{0xFF, 0xD8, 0x33, 0xFF, 0xD9}

	stream := append([]byte{0x00, 0x99, 0xFF, 0x00}, first...)
	stream = append(stream, second...)

	br := bufio.NewReader(bytes.NewReader(stream))

	got, err := nextJPEG(br)
	if err != nil {
		t.Fatalf("nextJPEG: %v", err)
	}

	if !bytes.Equal(got, first) {
		t.Errorf("first frame = %v, want %v", got, first)
	}

	got, err = nextJPEG(br)
	if err != nil {
		t.Fatalf("nextJPEG: %v", err)
	}

	if !bytes.Equal(got, second) {
		t.Errorf("second frame = %v, want %v", got, second)
	}
}

func TestStartCamFailsWithoutSource(t *testing.T) {
	t.Setenv("TERMCHAT_CAM_SOURCE", "/nonexistent/termchat-test.jpg")

	_, err := startCam("")
	if err == nil {
		t.Error("startCam accepted a missing camera source")
	}
}

// TestStartCameraAndPumpFeedsPreview guards against a regression where the
// camera pump was never started, so no frames reached the local preview.
func TestStartCameraAndPumpFeedsPreview(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not found; skipping camera pump test")
	}

	still := filepath.Join(t.TempDir(), "still.jpg")
	raw := encodeTestJPEG(t, solidNRGBA(64, 48, color.NRGBA{R: 10, G: 200, B: 40, A: 255}))

	err := os.WriteFile(still, raw, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("TERMCHAT_CAM_SOURCE", still)

	cam, err := startCam("")
	if err != nil {
		t.Fatal(err)
	}
	defer cam.stop()

	vs := &VideoSession{
		conn: &MediaConn{
			outbox: make(chan []byte, 8),
			done:   make(chan struct{}),
		},
		done:  make(chan struct{}),
		peers: map[uint32]*videoPeerFrame{},
	}

	startCameraAndPump(vs, cam)

	deadline := time.Now().Add(5 * time.Second)

	for {
		vs.mu.Lock()
		self := vs.self
		vs.mu.Unlock()

		if self != nil {
			break
		}

		if time.Now().After(deadline) {
			t.Fatal("camera pump produced no self-preview frame")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

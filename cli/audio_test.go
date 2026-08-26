package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"termchat/shared"
)

func TestMixIntoSaturates(t *testing.T) {
	dst := []int16{100, 32767, -32768, 0}
	src := []int16{200, 1, -1, 0}

	mixInto(dst, src)

	want := []int16{300, 32767, -32768, 0}

	for i := range want {
		if dst[i] != want[i] {
			t.Fatalf("mix[%d] = %d, want %d", i, dst[i], want[i])
		}
	}
}

func TestBytesSamplesRoundtrip(t *testing.T) {
	samples := []int16{0, 1, -1, 32767, -32768, 12345}

	encoded := samplesToBytes(samples)
	decoded := bytesToSamples(encoded)

	if len(decoded) != len(samples) {
		t.Fatalf("len = %d, want %d", len(decoded), len(samples))
	}

	for i := range samples {
		if decoded[i] != samples[i] {
			t.Errorf("sample[%d] = %d, want %d", i, decoded[i], samples[i])
		}
	}
}

func TestChunkRingFIFOAndOverflow(t *testing.T) {
	r := &chunkRing{}

	if _, ok := r.pop(); ok {
		t.Fatal("empty ring popped a chunk")
	}

	first := []int16{1}
	second := []int16{2}

	r.push(first)
	r.push(second)

	got, ok := r.pop()
	if !ok || !bytes.Equal(samplesToBytes(got), samplesToBytes(first)) {
		t.Fatalf("pop = %v, want the first chunk", got)
	}

	overflow := &chunkRing{}

	for i := 0; i < ringCapacity+2; i++ {
		overflow.push([]int16{int16(i)})
	}

	if len(overflow.chunks) != ringCapacity {
		t.Fatalf("depth = %d, want capped at %d", len(overflow.chunks), ringCapacity)
	}

	got, _ = overflow.pop()

	if got[0] != int16(2) {
		t.Errorf("oldest surviving chunk = %d, want 2 (drop-oldest)", got[0])
	}
}

func TestChunkRingStale(t *testing.T) {
	r := &chunkRing{}
	r.push([]int16{9})

	now := time.Now()

	if r.stale(now) {
		t.Fatal("fresh ring reported stale")
	}

	if !r.stale(now.Add(ringStaleAfter + time.Second)) {
		t.Fatal("old ring not reported stale")
	}
}

func TestVoiceDumpsWriteHeadersAndChunks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMCHAT_VOICE_DEBUG", dir)

	d, err := openVoiceDumps()
	if err != nil {
		t.Fatal(err)
	}

	pid := os.Getpid()

	tx, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("tx-%d.wav", pid)))
	if err != nil {
		t.Fatal(err)
	}

	if len(tx) != 44 {
		t.Fatalf("tx dump = %d bytes at creation, want the 44-byte header", len(tx))
	}

	d.writeTX([]byte{1, 2, 3})
	d.writeRX([]byte{4, 5, 6})

	rxPath := filepath.Join(dir, fmt.Sprintf("rx-%d.wav", pid))

	stat, err := os.Stat(rxPath)
	if err != nil {
		t.Fatal(err)
	}

	if stat.Size() != 47 {
		t.Errorf("rx dump = %d bytes, want header + 3", stat.Size())
	}

	d.close()

	if _, err := d.tx.Stat(); err == nil {
		t.Error("tx dump still open after close")
	}
}

func TestStderrTailKeepsLastBytes(t *testing.T) {
	tail := newStderrTail(8)

	chunk := []byte("0123456789abcdef")
	n, err := tail.Write(chunk)
	if err != nil || n != len(chunk) {
		t.Fatalf("write = %d, %v; want %d, nil", n, err, len(chunk))
	}

	got := tail.String()

	if got != "89abcdef" {
		t.Errorf("tail = %q, want the last 8 bytes", got)
	}
}

func TestChunkPeak(t *testing.T) {
	cases := []struct {
		in   []int16
		want int16
	}{
		{[]int16{0, 0, 0}, 0},
		{[]int16{10, -500, 3}, 500},
		{[]int16{-32768}, 32767},
		{[]int16{32767, -8}, 32767},
		{[]int16{-1, 1}, 1},
	}

	for _, c := range cases {
		if got := chunkPeak(c.in); got != c.want {
			t.Errorf("chunkPeak(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestResolvePlayerOverride(t *testing.T) {
	t.Setenv("TERMCHAT_VOICE_PLAYER", "ffplay")

	kind, err := resolvePlayer()
	if err != nil || kind != playerFfplay {
		t.Fatalf("ffplay override = %v, %v", kind, err)
	}

	t.Setenv("TERMCHAT_VOICE_PLAYER", "paplay")

	kind, err = resolvePlayer()
	if err != nil || kind != playerPaplay {
		t.Fatalf("paplay override = %v, %v", kind, err)
	}

	t.Setenv("TERMCHAT_VOICE_PLAYER", "bogus")

	if _, err := resolvePlayer(); err == nil {
		t.Fatal("bogus override accepted")
	}
}

func TestPlayerSpecHeaderDecision(t *testing.T) {
	bin, _, header := playerSpec(playerFfplay)

	if bin != "ffplay" {
		t.Errorf("bin = %q, want ffplay", bin)
	}

	if len(header) != 44 {
		t.Fatalf("ffplay header = %d bytes, want 44", len(header))
	}

	pBin, pArgs, pHeader := playerSpec(playerPaplay)

	if pBin != "paplay" {
		t.Errorf("bin = %q, want paplay", pBin)
	}

	if pHeader != nil {
		t.Error("paplay must consume raw PCM without a header")
	}

	raw := false

	for _, a := range pArgs {
		if a == "--raw" {
			raw = true
		}
	}

	if !raw {
		t.Errorf("paplay args missing --raw: %v", pArgs)
	}
}

func TestPlayerCommandMissingBinary(t *testing.T) {
	if _, err := exec.LookPath("ffplay"); err == nil {
		t.Skip("ffplay installed; the missing-binary path is not reachable")
	}

	_, _, _, err := playerCommand(playerFfplay)
	if err == nil || !strings.Contains(err.Error(), "ffplay") {
		t.Fatalf("err = %v, want an ffplay-specific message", err)
	}
}

func TestWAVHeaderDescribesVoiceFormat(t *testing.T) {
	h := wavHeader()

	if len(h) != 44 {
		t.Fatalf("header = %d bytes, want 44", len(h))
	}

	if string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" || string(h[12:16]) != "fmt " || string(h[36:40]) != "data" {
		t.Errorf("magic fields wrong: %q %q %q %q", h[0:4], h[8:12], h[12:16], h[36:40])
	}

	if binary.LittleEndian.Uint32(h[24:]) != shared.AudioSampleRate {
		t.Errorf("sample rate = %d, want %d", binary.LittleEndian.Uint32(h[24:]), shared.AudioSampleRate)
	}

	byteRate := shared.AudioSampleRate * uint32(shared.AudioChannels) * 2
	if binary.LittleEndian.Uint32(h[28:]) != byteRate {
		t.Errorf("byte rate = %d, want %d", binary.LittleEndian.Uint32(h[28:]), byteRate)
	}

	if binary.LittleEndian.Uint16(h[22:]) != uint16(shared.AudioChannels) {
		t.Errorf("channels = %d, want %d", binary.LittleEndian.Uint16(h[22:]), shared.AudioChannels)
	}

	if binary.LittleEndian.Uint16(h[34:]) != 16 {
		t.Errorf("bits per sample = %d, want 16", binary.LittleEndian.Uint16(h[34:]))
	}

	if binary.LittleEndian.Uint16(h[20:]) != 1 {
		t.Errorf("format tag = %d, want PCM (1)", binary.LittleEndian.Uint16(h[20:]))
	}
}

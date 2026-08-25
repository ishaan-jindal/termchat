package main

import (
	"bytes"
	"testing"
	"time"
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

package main

import (
	"testing"
	"time"
)

// TestVideoPruneDropsStale verifies prune removes frames older than the window.
func TestVideoPruneDropsStale(t *testing.T) {
	vs := &VideoSession{
		done:  make(chan struct{}),
		peers: map[uint32]*videoPeerFrame{},
	}

	vs.peers[1] = &videoPeerFrame{pix: solidRGB(4, 4, 1, 2, 3), w: 4, h: 4, updated: time.Now()}
	vs.peers[2] = &videoPeerFrame{pix: solidRGB(4, 4, 4, 5, 6), w: 4, h: 4, updated: time.Now().Add(-10 * time.Second)}

	vs.prune()

	if _, ok := vs.peers[2]; ok {
		t.Error("prune kept a stale peer")
	}

	if _, ok := vs.peers[1]; !ok {
		t.Error("prune dropped a fresh peer")
	}
}

// TestVideoEvictOldestDropsOldest verifies the peer cap evicts the oldest stream.
func TestVideoEvictOldestDropsOldest(t *testing.T) {
	vs := &VideoSession{
		done:  make(chan struct{}),
		peers: map[uint32]*videoPeerFrame{},
	}

	base := time.Now()

	for id := uint32(1); id <= maxVideoPeers; id++ {
		vs.peers[id] = &videoPeerFrame{pix: solidRGB(2, 2, 1, 1, 1), w: 2, h: 2, updated: base.Add(time.Duration(id) * time.Second)}
	}

	vs.mu.Lock()
	vs.evictOldest()
	vs.mu.Unlock()

	if len(vs.peers) != maxVideoPeers-1 {
		t.Fatalf("peers = %d, want %d", len(vs.peers), maxVideoPeers-1)
	}

	if _, ok := vs.peers[1]; ok {
		t.Error("evictOldest kept the oldest peer")
	}
}

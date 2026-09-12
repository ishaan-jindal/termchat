package main

import (
	"strings"
	"testing"

	"termchat/shared"
)

// TestVCJoinRequestsToken verifies a bare /vc with no media conn requests a
// token and marks both sessions as wanted.
func TestVCJoinRequestsToken(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/vc")
	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	if !m.wantVoice || !m.wantVideo {
		t.Errorf("wantVoice=%v wantVideo=%v, want both true", m.wantVoice, m.wantVideo)
	}

	if !m.tokenPending {
		t.Error("tokenPending = false, want true")
	}

	sent := drainSend(t, m)
	if len(sent) != 1 || sent[0].Type != "media_token" {
		t.Errorf("sent = %+v, want a single media_token", sent)
	}

	if m.pendingCmd == nil {
		t.Error("pendingCmd not armed for the token timeout")
	}
}

// TestVCAttachesBothWhenMediaExists verifies /vc with an existing media conn
// starts both sessions and leaves the camera off.
func TestVCAttachesBothWhenMediaExists(t *testing.T) {
	m := testModel()
	m.media = &MediaConn{
		outbox: make(chan []byte, 8),
		audio:  make(chan []byte, 8),
		video:  make(chan []byte, 8),
		done:   make(chan struct{}),
		closed: make(chan struct{}),
	}

	handled, _ := handleCommand(&m, "/vc")
	if !handled {
		t.Fatal("/vc not handled")
	}

	if m.pendingCmd == nil {
		t.Fatal("pendingCmd not armed with the session start batch")
	}

	// Executing the batch attaches the video session in watch mode (camera
	// off). Voice needs a real player, so only the video side is asserted.
	m.pendingCmd()

	if m.video == nil {
		t.Fatal("video session not attached by the join batch")
	}

	if m.video.tx {
		t.Error("video session must join with the camera off")
	}
}

// TestVCTogglesToLeave verifies a second bare /vc while in the call leaves.
func TestVCTogglesToLeave(t *testing.T) {
	m := testModel()
	m.video = &VideoSession{done: make(chan struct{}), peers: map[uint32]*videoPeerFrame{}}

	if !inVC(&m) {
		t.Fatal("expected to be in vc")
	}

	handled, _ := handleCommand(&m, "/vc")
	if !handled {
		t.Fatal("/vc not handled")
	}

	if inVC(&m) {
		t.Error("still in vc after leave toggle")
	}

	if !strings.Contains(lastUI(m), "left vc") {
		t.Errorf("leave message missing: %q", lastUI(m))
	}
}

// TestVCOffOutsideGuidance verifies Ctrl+T / Ctrl+V and /vc off guidance.
func TestVCOffAndKeyGuidance(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/vc off")
	if !handled {
		t.Fatal("/vc off not handled")
	}

	if m.pendingCmd != nil {
		t.Error("leaving an empty vc should not arm a command")
	}

	// Ctrl+T outside vc prints guidance and makes no state change.
	m2 := testModel()
	toggleTalk(&m2)
	if !strings.Contains(lastUI(m2), "/vc") {
		t.Errorf("Ctrl+T guidance missing /vc: %q", lastUI(m2))
	}

	// Ctrl+V outside vc prints guidance.
	m3 := testModel()
	toggleVideo(&m3)
	if !strings.Contains(lastUI(m3), "/vc") {
		t.Errorf("Ctrl+V guidance missing /vc: %q", lastUI(m3))
	}
}

// TestVCDupJoin verifies an explicit /vc on while in the call is rejected.
func TestVCDupJoin(t *testing.T) {
	m := testModel()
	m.video = &VideoSession{done: make(chan struct{}), peers: map[uint32]*videoPeerFrame{}}

	handled, _ := handleCommand(&m, "/vc on")
	if !handled {
		t.Fatal("/vc on not handled")
	}

	if m.pendingCmd != nil {
		t.Error("duplicate join should not arm a command")
	}

	if !strings.Contains(lastUI(m), "already in vc") {
		t.Errorf("duplicate join message missing: %q", lastUI(m))
	}
}

// lastUI returns the most recent local feedback line rendered into the log.
func lastUI(m Model) string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if m.messages[i].kind == lineUI {
			return m.messages[i].msg.Text
		}
	}

	return ""
}

// TestVCProtocolUnchanged guards that the shared media kinds still exist.
func TestVCProtocolUnchanged(t *testing.T) {
	if shared.MediaKindVideo == 0 {
		t.Error("MediaKindVideo should be nonzero")
	}
}

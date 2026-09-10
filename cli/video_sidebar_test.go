package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// fakeVideoSession builds a VideoSession with a self-preview and the given
// peer frames, ready for sidebar rendering.
func fakeVideoSession(self bool, peerPix []byte) *VideoSession {
	vs := &VideoSession{
		tx:    self,
		done:  make(chan struct{}),
		peers: map[uint32]*videoPeerFrame{},
	}

	if self {
		vs.self = &videoPeerFrame{pix: solidRGB(16, 12, 20, 40, 60), w: 16, h: 12, updated: now()}
	}

	if peerPix != nil {
		vs.peers[1] = &videoPeerFrame{pix: peerPix, w: 16, h: 12, updated: now()}
	}

	return vs
}

func now() time.Time {
	return time.Now()
}

func sidebarModel(width, height int, vs *VideoSession) Model {
	m := testModel()

	m.width = width
	m.height = height
	m.compactMode = width < 100
	m.showSidebar = width >= 70
	m.video = vs

	m.users = []UserInfo{
		{Nick: "bob", Color: "#00ff00"},
	}

	fitWidths(&m)
	resizeViewport(&m)

	return m
}

func TestSidebarWidthExpandsWithVideo(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, nil))
	if w := m.sidebarWidth(); w != 30 {
		t.Errorf("sidebar width with video = %d, want 30", w)
	}

	m2 := sidebarModel(120, 40, nil)
	if w := m2.sidebarWidth(); w != 22 {
		t.Errorf("sidebar width without video = %d, want 22", w)
	}
}

func TestVideoColumnVisibleTransitions(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, nil))
	if !m.videoColumnVisible() {
		t.Error("video column should be visible while transmitting")
	}

	m.video.stopTx()
	if m.videoColumnVisible() {
		t.Error("video column should hide when not transmitting and no peers")
	}
}

func TestSidebarShowsVideoColumnAboveRoster(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, nil))
	m.users = []UserInfo{{Nick: "bob", Color: "#00ff00"}}

	view := renderSidebar(m, m.viewport.Height)

	if !strings.Contains(view, "VIDEO") {
		t.Error("sidebar missing VIDEO header")
	}

	if !strings.Contains(view, "USERS") {
		t.Error("sidebar missing USERS header")
	}

	videoIdx := strings.Index(view, "VIDEO")
	usersIdx := strings.Index(view, "USERS")

	if videoIdx < 0 || usersIdx < 0 || videoIdx > usersIdx {
		t.Errorf("VIDEO must appear above USERS (VIDEO@%d USERS@%d)", videoIdx, usersIdx)
	}
}

func TestSidebarTileShowsSelfCaption(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, nil))
	m.nick = "alice"

	view := renderSidebar(m, m.viewport.Height)

	if !strings.Contains(view, "alice (you)") {
		t.Error("self tile caption missing '(you)' marker")
	}
}

func TestSidebarTileShowsHostFlag(t *testing.T) {
	forceColor(t)

	vs := fakeVideoSession(false, solidRGB(16, 12, 30, 50, 70))
	m := sidebarModel(120, 40, vs)
	m.users = []UserInfo{{Nick: "bob", Color: "#00ff00", IsHost: true, VoiceID: 1}}

	view := renderSidebar(m, m.viewport.Height)

	if !strings.Contains(view, "bob [host]") {
		t.Errorf("peer tile caption missing host flag: %q", view)
	}
}

func TestSidebarOverflowLine(t *testing.T) {
	forceColor(t)

	vs := fakeVideoSession(true, solidRGB(16, 12, 30, 50, 70))

	// Fill peers past the tile capacity so only some fit, forcing overflow.
	for id := uint32(1); id <= 6; id++ {
		vs.peers[id] = &videoPeerFrame{pix: solidRGB(16, 12, 30, 50, 70), w: 16, h: 12, updated: now()}
	}

	m := sidebarModel(120, 20, vs)
	m.nick = "alice"
	m.users = []UserInfo{
		{Nick: "bob", Color: "#00ff00"},
		{Nick: "carol", Color: "#00ff00"},
		{Nick: "dave", Color: "#00ff00"},
		{Nick: "erin", Color: "#00ff00"},
		{Nick: "frank", Color: "#00ff00"},
		{Nick: "grace", Color: "#00ff00"},
	}

	view := renderSidebar(m, m.viewport.Height)

	if !strings.Contains(view, "more on video") {
		t.Errorf("overflow line missing when tiles exceed space: %q", view)
	}
}

func TestFooterShowsOnVideoCount(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, solidRGB(16, 12, 30, 50, 70)))
	m.nick = "alice"

	view := m.View()

	if !strings.Contains(view, "on video") {
		t.Errorf("footer missing on-video count: %q", view)
	}
}

func TestVideoPanelHiddenWhenSidebarShown(t *testing.T) {
	forceColor(t)

	m := sidebarModel(120, 40, fakeVideoSession(true, nil))

	if h := m.videoPanelHeight(); h != 0 {
		t.Errorf("videoPanelHeight = %d, want 0 when sidebar shown", h)
	}
}

func TestVideoPanelShownWhenNarrow(t *testing.T) {
	forceColor(t)

	m := sidebarModel(60, 30, fakeVideoSession(true, nil))

	if h := m.videoPanelHeight(); h <= 0 {
		t.Errorf("videoPanelHeight = %d, want > 0 on narrow terminals", h)
	}
}

func TestFitWidthsParityWithoutVideo(t *testing.T) {
	forceColor(t)

	// 100x30, no video: matches the pre-video layout math.
	m := sidebarModel(100, 30, nil)

	if m.showSidebar != true {
		t.Errorf("showSidebar = %v, want true", m.showSidebar)
	}

	if m.viewport.Width != 68 {
		t.Errorf("viewport width = %d, want 68", m.viewport.Width)
	}
}

// stressSidebarModel builds a sidebar with a self tile, several peers and
// long roster statuses so every width-bounding path is exercised.
func stressSidebarModel(width, height int) Model {
	vs := fakeVideoSession(true, solidRGB(16, 12, 30, 50, 70))

	// Several peers plus the self tile push the tile loop and overflow.
	for id := uint32(1); id <= 5; id++ {
		vs.peers[id] = &videoPeerFrame{pix: solidRGB(16, 12, 30, 50, 70), w: 16, h: 12, updated: now()}
	}

	m := sidebarModel(width, height, vs)
	m.nick = "alice"

	m.users = []UserInfo{
		{Nick: "alice", Color: "#ff0000", IsHost: true, Typing: true},
		{Nick: "bob", Color: "#00ff00", VoiceID: 1, IsHost: true, Typing: true},
		{Nick: "carol", Color: "#0000ff", VoiceID: 2, Typing: true},
		{Nick: "dave", Color: "#ffff00", VoiceID: 3},
		{Nick: "erin", Color: "#ff00ff", VoiceID: 4},
		{Nick: "frank", Color: "#00ffff", VoiceID: 5},
	}

	return m
}

// assertSidebarLinesFit verifies the rendered sidebar is a clean rectangle:
// every content line fits the panel width and no nested tile box is
// fragmented across lines (a split corner would leave a lone ╭ or ╰).
func assertSidebarLinesFit(t *testing.T, m Model) {
	t.Helper()

	width := m.sidebarWidth()
	content := renderSidebar(m, m.viewport.Height)

	for i, line := range strings.Split(content, "\n") {
		plain := ansi.Strip(line)

		if w := ansi.StringWidth(plain); w != width {
			t.Errorf("sidebar line %d width = %d, want %d: %q", i, w, width, line)
		}

		for _, corner := range []string{"╭", "╰"} {
			if strings.Contains(plain, corner) && !strings.Contains(plain, boxOpposite(corner)) {
				t.Errorf("sidebar line %d has a fragmented %s corner: %q", i, corner, line)
			}
		}
	}
}

func boxOpposite(corner string) string {
	if corner == "╭" {
		return "╮"
	}

	return "╯"
}

func TestSidebarLinesFitWide(t *testing.T) {
	forceColor(t)

	m := stressSidebarModel(120, 40)
	assertSidebarLinesFit(t, m)
}

func TestSidebarLinesFitCompact(t *testing.T) {
	forceColor(t)

	m := stressSidebarModel(80, 30)
	assertSidebarLinesFit(t, m)
}

// TestViewLinesMatchTerminalWidth renders the full chat View with an active
// video sidebar and asserts every display line is exactly the terminal
// width. Any lipgloss re-wrap or content overflow would change line widths.
func TestViewLinesMatchTerminalWidth(t *testing.T) {
	forceColor(t)

	for _, size := range [][2]int{{120, 40}, {80, 30}} {
		m := stressSidebarModel(size[0], size[1])
		view := m.View()

		for i, line := range strings.Split(view, "\n") {
			plain := ansi.Strip(line)

			if w := ansi.StringWidth(plain); w != size[0] {
				t.Errorf("[%dx%d] view line %d width = %d, want %d: %q", size[0], size[1], i, w, size[0], line)
			}
		}
	}
}

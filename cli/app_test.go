package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"termchat/shared"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

func testHub(serverURL, base, room string, fresh bool) appModel {
	return newAppModel(hubOptions{
		room:      room,
		fresh:     fresh,
		serverURL: serverURL,
		base:      base,
		port:      8080,
		cfg:       Config{Nick: "alice"},
		theme:     registeredTheme("dark"),
	})
}

func updateApp(t *testing.T, a appModel, msg tea.Msg) (appModel, tea.Cmd) {
	t.Helper()

	next, cmd := a.Update(msg)

	app, ok := next.(appModel)
	if !ok {
		t.Fatalf("Update returned %T, want appModel", next)
	}

	return app, cmd
}

func closeJoined(t *testing.T, msg tea.Msg) *Connection {
	t.Helper()

	joined, ok := msg.(joinedMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want joinedMsg", msg)
	}

	t.Cleanup(func() {
		close(joined.conn.done)
		joined.conn.conn.Close()
	})

	return joined.conn
}

func TestHubPrefills(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "FROG", false)

	if a.room.Value() != "FROG" {
		t.Errorf("room = %q, want FROG", a.room.Value())
	}

	if a.nick.Value() != "alice" {
		t.Errorf("nick = %q, want alice", a.nick.Value())
	}

	if a.focus != focusNick {
		t.Errorf("focus = %d, want nick field", a.focus)
	}

	if !a.nick.Focused() {
		t.Error("nick field not focused")
	}
}

func TestHubDiscoverFocusesForm(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)

	if a.focus != focusNick {
		t.Errorf("focus = %d, want nick field", a.focus)
	}
}

func TestHubTabSkipsEmptyList(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)
	a.focus = focusPass

	// With no rows the list is unreachable; tab wraps to the room field.
	a, _ = updateApp(t, a, tea.KeyMsg{Type: tea.KeyTab})

	if a.focus != focusRoom {
		t.Errorf("focus = %d, want room field (list skipped)", a.focus)
	}

	// Once a row appears, tab reaches the list from the room field.
	a, _ = updateApp(t, a, onlineRoomsMsg{rooms: []shared.RoomInfo{{ID: "ABCD"}}})

	for i := 0; i < 3; i++ {
		a, _ = updateApp(t, a, tea.KeyMsg{Type: tea.KeyTab})
	}

	if a.focus != focusList {
		t.Errorf("focus = %d, want list after rows appear", a.focus)
	}
}

func TestHubScanEmptyListBouncesToNick(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)
	a.focus = focusList

	a, cmd := updateApp(t, a, onlineRoomsMsg{rooms: []shared.RoomInfo{}})

	if a.focus != focusNick {
		t.Errorf("focus = %d, want nick after list emptied", a.focus)
	}

	if cmd == nil {
		t.Error("bounce should refocus the nick field")
	}
}

func TestHubCtrlHHosts(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "FROG", false)

	a, cmd := updateApp(t, a, tea.KeyMsg{Type: tea.KeyCtrlH})

	if cmd == nil {
		t.Fatal("ctrl+h from a form field did not host")
	}

	if !a.busy {
		t.Error("hub not busy after ctrl+h")
	}
}

func TestHubCtrlTThemeCycles(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "FROG", false)
	before := a.theme.Name

	a, cmd := updateApp(t, a, tea.KeyMsg{Type: tea.KeyCtrlT})

	if cmd != nil {
		t.Fatalf("ctrl+t returned a cmd, want nil: %v", cmd)
	}

	if a.theme.Name == before {
		t.Errorf("theme did not cycle: %q", before)
	}

	if a.cfg.Theme != a.theme.Name {
		t.Errorf("cfg.Theme = %q, want %q", a.cfg.Theme, a.theme.Name)
	}
}

func TestHubFreshRoomFocusesRoom(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "ABCD", true)

	if a.focus != focusRoom {
		t.Errorf("focus = %d, want room field", a.focus)
	}
}

func TestHubInvalidNickBlocked(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "FROG", false)
	a.nick.SetValue("bad name")

	a.startJoin(a.serverURL)

	if a.busy {
		t.Error("startJoin dialed with an invalid nick")
	}

	if !strings.Contains(a.errLine, "spaces are not allowed") {
		t.Errorf("errLine = %q, want nick reason", a.errLine)
	}

	if a.focus != focusNick {
		t.Errorf("focus = %d, want nick field", a.focus)
	}
}

func TestHubInvalidRoomBlocked(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "ABC", false)

	a.startJoin(a.serverURL)

	if a.busy {
		t.Error("startJoin dialed with an invalid room")
	}

	if !strings.Contains(a.errLine, "invalid room code") {
		t.Errorf("errLine = %q, want room error", a.errLine)
	}

	if a.focus != focusRoom {
		t.Errorf("focus = %d, want room field", a.focus)
	}
}

func TestHubBlankNickFallsBack(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	a := testHub(url, "http://"+addr, "HUB1", false)
	a.nick.SetValue("")

	cmd := a.startJoin(a.serverURL)
	if cmd == nil {
		t.Fatal("startJoin refused a blank nick")
	}

	closeJoined(t, cmd())

	if a.nick.Value() != "anonymous" {
		t.Errorf("nick = %q, want anonymous fallback", a.nick.Value())
	}
}

func TestHubJoinEntersChat(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	a := testHub(url, "http://"+addr, "HUB1", true)

	a, _ = updateApp(t, a, tea.WindowSizeMsg{Width: 100, Height: 30})

	cmd := a.startJoin(a.serverURL)
	if cmd == nil {
		t.Fatal("startJoin refused a valid form")
	}

	if !a.busy {
		t.Error("hub not busy while joining")
	}

	msg := cmd()
	conn := closeJoined(t, msg)

	a, _ = updateApp(t, a, msg)

	if a.screen != screenChat {
		t.Fatal("hub did not switch to the chat screen")
	}

	if a.chat.nick != "alice" || a.chat.room != "HUB1" {
		t.Errorf("chat identity = %s/%s, want alice/HUB1", a.chat.nick, a.chat.room)
	}

	if a.conn != conn {
		t.Error("shell did not track the live connection")
	}

	if a.chat.viewport.Width != 68 {
		t.Errorf("chat viewport width = %d, want 68", a.chat.viewport.Width)
	}

	// No share line is appended on entry.
	if len(a.chat.messages) != 0 {
		t.Fatalf("chat has %d lines, want none (no share line)", len(a.chat.messages))
	}
}

func TestHubLockedRoomAsksPassword(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	host, err := connectWebSocket(url)
	if err != nil {
		t.Fatal(err)
	}

	go writePump(host)

	if _, err := joinOnce(host, "HUB2", "host", "", ""); err != nil {
		t.Fatal(err)
	}

	host.Send <- Message{Type: "set_password", Password: "secret"}

	if err := waitForLocked(addr, "HUB2"); err != nil {
		t.Fatal(err)
	}

	defer func() {
		close(host.done)
		host.conn.Close()
	}()

	a := testHub(url, "http://"+addr, "HUB2", false)

	msg := joinCmd(url, "HUB2", "alice", "", "")()
	if _, ok := msg.(joinPasswordMsg); !ok {
		t.Fatalf("join returned %T, want joinPasswordMsg", msg)
	}

	a, _ = updateApp(t, a, msg)

	if a.busy {
		t.Error("hub still busy after the password rejection")
	}

	if !a.passRequired {
		t.Error("hub did not arm the inline password prompt")
	}

	if a.errLine != "" {
		t.Errorf("errLine = %q, want empty (prompt is inline)", a.errLine)
	}

	if a.focus != focusPass {
		t.Errorf("focus = %d, want password field", a.focus)
	}

	// The hint renders inline on the password field row.
	a, _ = updateApp(t, a, tea.WindowSizeMsg{Width: 100, Height: 30})

	if view := a.View(); !strings.Contains(view, "enter password") {
		t.Errorf("hub view missing the inline password hint:\n%s", view)
	}

	a.pass.SetValue("secret")
	join := a.startJoin(a.serverURL)

	if join == nil {
		t.Fatal("hub refused to retry with a password")
	}

	joined := join()
	closeJoined(t, joined)

	a, _ = updateApp(t, a, joined)

	if a.screen != screenChat {
		t.Fatal("hub did not enter chat after the password retry")
	}
}

// TestHubLockedLANRoomAsksPassword drives the same password flow through a
// locked LAN row: the beacon advertises locked, the join is rejected, and
// the hub asks for the password in place.
func TestHubLockedLANRoomAsksPassword(t *testing.T) {
	addr := startRealServer(t)

	host, err := connectWebSocket("ws://" + addr + "/ws")
	if err != nil {
		t.Fatal(err)
	}

	go writePump(host)

	if _, err := joinOnce(host, "HUB5", "host", "", ""); err != nil {
		t.Fatal(err)
	}

	host.Send <- Message{Type: "set_password", Password: "secret"}

	if err := waitForLocked(addr, "HUB5"); err != nil {
		t.Fatal(err)
	}

	defer func() {
		close(host.done)
		host.conn.Close()
	}()

	// The LAN row points at the real server on loopback.
	a := testHub("ws://example.test/ws", "http://example.test", "", false)
	a.lan = []lanBeacon{{Room: "HUB5", Host: "host", IP: "127.0.0.1", Port: lanPort(addr), Locked: true}}
	a.rebuildRows()
	a.focus = focusList

	if len(a.rows) != 1 || !a.rows[0].locked {
		t.Fatalf("rows = %+v, want one locked lan row", a.rows)
	}

	cmd := a.joinRow(a.rows[0])

	msg := cmd()
	if _, ok := msg.(joinPasswordMsg); !ok {
		t.Fatalf("lan join returned %T, want joinPasswordMsg", msg)
	}

	a, _ = updateApp(t, a, msg)

	if !a.passRequired {
		t.Error("hub did not arm the inline password prompt")
	}

	if a.errLine != "" {
		t.Errorf("errLine = %q, want empty (prompt is inline)", a.errLine)
	}

	if a.focus != focusPass {
		t.Errorf("focus = %d, want password field", a.focus)
	}

	// The password field carries into the retry against the LAN host.
	a.pass.SetValue("secret")
	join := a.startJoin(a.serverURL)

	if join == nil {
		t.Fatal("hub refused to retry with a password")
	}

	joined := join()
	closeJoined(t, joined)

	a, _ = updateApp(t, a, joined)

	if a.screen != screenChat {
		t.Fatal("hub did not enter chat after the LAN password retry")
	}
}

func lanPort(addr string) int {
	idx := strings.LastIndex(addr, ":")

	if idx < 0 {
		return 0
	}

	var port int

	fmt.Sscanf(addr[idx+1:], "%d", &port)

	return port
}

func TestHubJoinErrorSurfaces(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "FROG", false)
	a.busy = true

	a, _ = updateApp(t, a, joinErrorMsg{err: errors.New("boom")})

	if a.busy {
		t.Error("hub still busy after the join error")
	}

	if a.errLine != "boom" {
		t.Errorf("errLine = %q, want boom", a.errLine)
	}
}

func TestHubScanBuildsRows(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "", false)

	a, _ = updateApp(t, a, onlineRoomsMsg{rooms: []shared.RoomInfo{
		{ID: "ABCD", HostNick: "alice", UserCount: 2},
		{ID: "EFGH", HostNick: "bob", UserCount: 1, HasPassword: true},
	}})

	if len(a.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(a.rows))
	}

	a, _ = updateApp(t, a, lanRoomsMsg{beacons: []lanBeacon{
		{Room: "IJKL", Host: "laptop", IP: "192.168.1.42", Port: 8080},
		{Room: "LMNO", Host: "phone", IP: "192.168.1.43", Port: 8080, Locked: true},
	}})

	if len(a.rows) != 4 {
		t.Fatalf("rows = %d, want 4", len(a.rows))
	}

	if a.rows[2].online || a.rows[2].addr != "192.168.1.42" {
		t.Errorf("lan row = %+v, want the beacon address", a.rows[2])
	}

	if !a.rows[3].locked {
		t.Errorf("lan row = %+v, want the locked flag", a.rows[3])
	}

	a, _ = updateApp(t, a, tea.WindowSizeMsg{Width: 100, Height: 30})

	view := a.View()

	for _, want := range []string{"termchat", "ABCD", "EFGH", "IJKL", "LMNO", "[locked]", "192.168.1.42"} {
		if !strings.Contains(view, want) {
			t.Errorf("hub view missing %q", want)
		}
	}
}

func TestHubEnterOnRowJoins(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	a := testHub(url, "http://"+addr, "", false)
	a.rows = []hubRow{{online: true, room: "HUB3", host: "x"}}
	a.focus = focusList

	a, cmd := updateApp(t, a, tea.KeyMsg{Type: tea.KeyEnter})

	if cmd == nil {
		t.Fatal("enter on a discover row did not join")
	}

	closeJoined(t, cmd())

	if a.room.Value() != "HUB3" {
		t.Errorf("room = %q, want HUB3 filled from the row", a.room.Value())
	}
}

func TestHubJoinCmdNickRejected(t *testing.T) {
	addr := startRealServer(t)
	url := "ws://" + addr + "/ws"

	msg := joinCmd(url, "HUB4", "bad name", "", "")()

	failed, ok := msg.(joinErrorMsg)
	if !ok {
		t.Fatalf("join returned %T, want joinErrorMsg", msg)
	}

	if !strings.Contains(failed.err.Error(), "nickname rejected") {
		t.Errorf("err = %q, want the nick rejection", failed.err)
	}
}

func TestHubHeaderFooter(t *testing.T) {
	a := testHub("ws://example.test/ws", "http://example.test", "ABCD", true)

	next, _ := a.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	a = next.(appModel)

	view := a.View()

	for _, want := range []string{
		"termchat",
		hubVersion() + " - dark",
		"JOIN OR CREATE",
		"Room",
		"Nickname",
		"Password",
		"ONLINE ROOMS",
		"LAN ROOMS",
		"enter join - tab focus - ctrl+h host - ctrl+r rescan - ctrl+t theme",
		"\u256d", // rounded box top-left corner
	} {
		if !strings.Contains(view, want) {
			t.Errorf("hub view missing %q", want)
		}
	}

	// The box is centered: the first content line is left-padded.
	lines := strings.Split(view, "\n")

	found := false

	for _, line := range lines {
		if strings.Contains(line, "termchat") && strings.HasPrefix(line, " ") {
			found = true

			break
		}
	}

	if !found {
		t.Error("hub box not indented (centering padding missing)")
	}
}

// TestHubViewHasNoUnpaintedCells mirrors the chat's bleed matrix: every hub
// state must render with the theme background on every visible cell.
func TestHubViewHasNoUnpaintedCells(t *testing.T) {
	forceColor(t)

	type state struct {
		name  string
		build func(a *appModel)
	}

	states := []state{
		{"fresh", func(a *appModel) {}},
		{"typed", func(a *appModel) {
			a.room.SetValue("FROG")
			a.nick.SetValue("robert")
			a.pass.SetValue("hunter2")
		}},
		{"nick-cursor-mid", func(a *appModel) {
			a.nick.SetValue("robert")
			a.nick.SetCursor(3)
			a.focus = focusNick
		}},
		{"locked-prompt", func(a *appModel) {
			a.passRequired = true
			a.pass.SetValue("hunter2")
			a.focus = focusPass
		}},
		{"busy", func(a *appModel) {
			a.busy = true
			a.busyText = "Joining room FROG..."
		}},
		{"busy-ticked", func(a *appModel) {
			a.busy = true
			a.busyText = "Joining room FROG..."

			var cmd tea.Cmd

			a.spin, cmd = a.spin.Update(spinner.TickMsg{})
			_ = cmd
		}},
		{"rows-listed", func(a *appModel) {
			a.online = []shared.RoomInfo{
				{ID: "ABCD", HostNick: "alice", UserCount: 2},
				{ID: "EFGH", HostNick: "bob", UserCount: 1, HasPassword: true},
			}
			a.onlineDone = true
			a.lan = []lanBeacon{{Room: "IJKL", Host: "laptop", IP: "192.168.1.42", Port: 8080}}
			a.lanDone = true
			a.rebuildRows()
			a.focus = focusList
			a.sel = 1
		}},
		{"empty-sections", func(a *appModel) {
			a.onlineDone = true
			a.lanDone = true
			a.focus = focusList
		}},
		{"host-server", func(a *appModel) {
			a.hostMode = true
			a.port = 9000
			a.busy = true
			a.busyText = "Joining room FROG (self-hosted)..."
		}},
	}

	for _, s := range states {
		for _, blink := range []bool{false, true} {
			a := testHub("ws://example.test/ws", "http://example.test", "ABCD", true)

			next, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			a = next.(appModel)

			s.build(&a)
			a.room.Cursor.Blink = blink
			a.nick.Cursor.Blink = blink
			a.pass.Cursor.Blink = blink

			view := a.View()

			if idx := unpaintedRuneIndex(view); idx >= 0 {
				start := max(idx-50, 0)
				end := min(idx+30, len(view))

				t.Errorf("[%s blink=%v] unpainted cell at %d: %q", s.name, blink, idx, view[start:end])
			}

			bare := bareSpaceRun.FindStringIndex(view)
			if bare != nil {
				start := max(bare[0]-40, 0)
				end := min(bare[1]+20, len(view))

				t.Errorf("[%s blink=%v] plain spaces after reset at %d: %q", s.name, blink, bare[0], view[start:end])
			}
		}
	}
}

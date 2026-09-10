package main

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// bareSpaceRun matches any plain space directly after a reset sequence;
// every visible cell, including spaces, must carry the theme background.
var bareSpaceRun = regexp.MustCompile("\x1b\\[0m[ ]")

// TestViewStatesHaveNoUnpaintedCells walks the reachable UI states and fails
// if any visible rune renders without an active theme background.
func TestViewStatesHaveNoUnpaintedCells(t *testing.T) {
	forceColor(t)

	type state struct {
		name  string
		build func(m *Model)
	}

	states := []state{
		{"fresh", func(m *Model) {}},
		{"typed-end", func(m *Model) {
			m.input.SetValue("hello world")
		}},
		{"typed-mid-blink-on", func(m *Model) {
			m.input.SetValue("hello world")
			m.input.SetCursor(5)
			m.input.Cursor.Blink = true
		}},
		{"typed-mid-blink-off", func(m *Model) {
			m.input.SetValue("hello world")
			m.input.SetCursor(5)
			m.input.Cursor.Blink = false
		}},
		{"multiline", func(m *Model) {
			m.input.SetValue("line one\nline two")
			m.input.SetCursor(4)
		}},
		{"popup-open", func(m *Model) {
			m.input.SetValue("/")
			refreshCompletion(m)
		}},
		{"react-popup", func(m *Model) {
			m.input.SetValue("/react 3 fi")
			refreshCompletion(m)
		}},
		{"scrolled-up", func(m *Model) {
			m.autoScroll = false
			m.viewport.ScrollUp(5)
		}},
		{"mention-msg", func(m *Model) {
			appendFormattedMessage(m, Message{Type: "message", ID: 2, Nick: "bob", Color: "#00ff00", Text: "yo @alice check this"})
		}},
		{"reply-quote", func(m *Model) {
			appendFormattedMessage(m, Message{
				Type: "message", ID: 3, Nick: "bob", Color: "#00ff00",
				Text: "a reply here", ReplyToID: 1, ReplyToNick: "alice", ReplyToText: "the original text",
			})
		}},
		{"reactions", func(m *Model) {
			appendFormattedMessage(m, Message{
				Type: "message", ID: 4, Nick: "bob", Color: "#00ff00", Text: "react to me",
				Reactions: []Reaction{{Name: "+1", Count: 2}, {Name: "fire", Count: 1}},
			})
		}},
		{"system-lines", func(m *Model) {
			appendFormattedMessage(m, Message{Type: "system", Text: "bob joined the room"})
			appendUI(m, "Theme set to dracula")
		}},
		{"users-listed", func(m *Model) {
			m.usersRequested = true
			m.users = []UserInfo{
				{Nick: "alice", Color: "#ff0000", IsHost: true, Typing: true},
				{Nick: "bob", Color: "#00ff00"},
			}
			*m, _ = update(t, *m, IncomingMessage(Message{Type: "users_list"}))
		}},
		{"compact-mode", func(m *Model) {
			*m, _ = update(t, *m, tea.WindowSizeMsg{Width: 80, Height: 30})
		}},
		{"narrow-no-sidebar", func(m *Model) {
			*m, _ = update(t, *m, tea.WindowSizeMsg{Width: 60, Height: 24})
		}},
		{"host-status", func(m *Model) {
			m.IsHost = true
			m.HostIP = "192.168.1.42"
			m.HostPort = 9000
		}},
		{"long-wrapped", func(m *Model) {
			long := ""

			for i := 0; i < 30; i++ {
				long += fmt.Sprintf("word%d ", i)
			}

			appendFormattedMessage(m, Message{Type: "message", ID: 9, Nick: "bob", Color: "#00ff00", Text: long})
		}},
		{"help-shown", func(m *Model) {
			handleCommand(m, "/help")
		}},
		{"theme-list-shown", func(m *Model) {
			handleCommand(m, "/theme")
		}},
		{"video-sidebar", func(m *Model) {
			vs := fakeVideoSession(true, solidRGB(16, 12, 30, 50, 70))
			vs.peers[2] = &videoPeerFrame{pix: solidRGB(16, 12, 70, 30, 50), w: 16, h: 12, updated: time.Now()}
			m.video = vs
			m.nick = "alice"
			m.users = []UserInfo{
				{Nick: "alice", Color: "#ff0000", IsHost: true, Typing: true},
				{Nick: "bob", Color: "#00ff00", VoiceID: 1},
				{Nick: "carol", Color: "#0000ff", VoiceID: 2, Typing: true},
			}
			refitLayout(m)
		}},
		{"video-sidebar-compact", func(m *Model) {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
			*m = updated.(Model)
			vs := fakeVideoSession(true, solidRGB(16, 12, 30, 50, 70))
			m.video = vs
			m.nick = "alice"
			m.users = []UserInfo{
				{Nick: "alice", Color: "#ff0000", IsHost: true},
				{Nick: "bob", Color: "#00ff00", VoiceID: 1, Typing: true},
			}
			refitLayout(m)
		}},
		{"focused-stream", func(m *Model) {
			vs := fakeVideoSession(false, solidRGB(16, 12, 30, 50, 70))
			vs.peers[1] = &videoPeerFrame{pix: solidRGB(16, 12, 70, 30, 50), w: 16, h: 12, updated: time.Now()}
			m.video = vs
			m.nick = "alice"
			m.users = []UserInfo{{Nick: "bob", Color: "#00ff00", VoiceID: 1, IsHost: true}}
			m.focusNick = "bob"
			refitLayout(m)
		}},
		{"focused-waiting", func(m *Model) {
			m.nick = "alice"
			m.users = []UserInfo{{Nick: "bob", Color: "#00ff00"}}
			m.focusNick = "bob"
			refitLayout(m)
		}},
	}

	for _, th := range themeNames() {
		theme, err := resolveTheme(th)
		if err != nil {
			t.Fatalf("resolveTheme(%q): %v", th, err)
		}

		for _, s := range states {
			for _, blink := range []bool{false, true} {
				m := testModelTheme(theme)

				m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
				m, _ = update(t, m, IncomingMessage(Message{Type: "message", ID: 1, Nick: "carol", Color: "#0000ff", Text: "base message"}))

				s.build(&m)
				m.input.Cursor.Blink = blink

				view := m.View()

				if idx := unpaintedRuneIndex(view); idx >= 0 {
					start := max(idx-50, 0)
					end := min(idx+30, len(view))

					t.Errorf("[%s %s blink=%v] unpainted cell at %d: %q", th, s.name, blink, idx, view[start:end])
				}

				bare := bareSpaceRun.FindStringIndex(view)
				if bare != nil {
					start := max(bare[0]-40, 0)
					end := min(bare[1]+20, len(view))

					t.Errorf("[%s %s blink=%v] plain spaces after reset at %d: %q", th, s.name, blink, bare[0], view[start:end])
				}
			}
		}
	}
}

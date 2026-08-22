package main

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// forceColor makes lipgloss emit ANSI even without a TTY, so rendering
// assertions can inspect the escape codes.
func forceColor(t *testing.T) {
	t.Helper()

	lipgloss.SetColorProfile(termenv.TrueColor)
}

func testModel() Model {
	return NewModel(&Connection{
		Send: make(chan Message, 32),
		done: make(chan struct{}),
	}, "alice", "TEST")
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	model, cmd := m.Update(msg)

	return model.(Model), cmd
}

func drainSend(t *testing.T, m Model) []Message {
	t.Helper()

	var msgs []Message

	for {
		select {
		case msg := <-m.conn.Send:
			msgs = append(msgs, msg)
		default:
			return msgs
		}
	}
}

func TestCommandNick(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/nick robert")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	if m.nick != "robert" {
		t.Errorf("nick = %q, want robert", m.nick)
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "nick" || msgs[0].NewNick != "robert" {
		t.Fatalf("sent = %+v, want single nick message", msgs)
	}
}

func TestCommandNickWithoutArg(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/nick")

	if !handled {
		t.Fatal("expected handled")
	}

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, want no message", msgs)
	}
}

func TestCommandColor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	m := testModel()

	handled, quit := handleCommand(&m, "/color #ff00aa")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "color" || msgs[0].Color != "#ff00aa" {
		t.Fatalf("sent = %+v, want color message", msgs)
	}

	cfg := loadConfig()

	if cfg.Color != "#ff00aa" {
		t.Errorf("saved color = %q", cfg.Color)
	}
}

func TestCommandColorInvalid(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/color red")

	if !handled {
		t.Fatal("expected handled")
	}

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, want no message", msgs)
	}

	found := false

	for _, line := range m.messages {
		if strings.Contains(line.rendered, "Invalid hex color") {
			found = true
		}
	}

	if !found {
		t.Error("no invalid-color feedback shown")
	}
}

func TestCommandClear(t *testing.T) {
	m := testModel()
	m.messages = []chatLine{{rendered: "one"}, {rendered: "two"}}

	handled, _ := handleCommand(&m, "/clear")

	if !handled {
		t.Fatal("expected handled")
	}

	if len(m.messages) != 0 {
		t.Errorf("messages = %+v, want empty", m.messages)
	}
}

func TestCommandHelp(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/help")

	if !handled {
		t.Fatal("expected handled")
	}

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	for _, cmd := range []string{"/password", "/users", "/react"} {
		if !strings.Contains(m.messages[0].rendered, cmd) {
			t.Errorf("help text = %q, want %s listed", m.messages[0].rendered, cmd)
		}
	}
}

func TestCommandUsers(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/users")

	if !handled {
		t.Fatal("expected handled")
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "users" {
		t.Fatalf("sent = %+v, want a users request", msgs)
	}

	if !m.usersRequested {
		t.Fatal("usersRequested = false, want true")
	}

	m, _ = update(t, m, IncomingMessage{
		Type: "users_list",
		Users: []UserInfo{
			{Nick: "alice", IsHost: true},
			{Nick: "bob"},
		},
	})

	if m.usersRequested {
		t.Error("usersRequested should be cleared once the list is shown")
	}

	if len(m.messages) != 3 {
		t.Fatalf("messages = %d, want a header plus two users", len(m.messages))
	}

	rendered := strings.Join(renderedLines(&m), " ")

	for _, want := range []string{"Users (2):", "alice (host)", "bob"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("output = %q, want %q", rendered, want)
		}
	}
}

func TestUsersListWithoutRequestIsSilent(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage{
		Type:  "users_list",
		Users: []UserInfo{{Nick: "alice", IsHost: true}},
	})

	if len(m.users) != 1 {
		t.Fatalf("users = %d, want 1", len(m.users))
	}

	if len(m.messages) != 0 {
		t.Errorf("messages = %+v, want empty", m.messages)
	}
}

func TestCommandQuit(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/quit")

	if !handled || !quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}
}

func TestCommandPassword(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/password new secret")

	if !handled {
		t.Fatal("expected handled")
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "set_password" || msgs[0].Password != "new secret" {
		t.Fatalf("sent = %+v, want set_password with joined args", msgs)
	}

	// No argument removes the password.
	handled, _ = handleCommand(&m, "/password")

	msgs = drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "set_password" || msgs[0].Password != "" {
		t.Fatalf("sent = %+v, want set_password with empty password", msgs)
	}
}

func TestCommandUnknownNotHandled(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/nosuchcommand")

	if handled {
		t.Fatal("unknown command should not be handled")
	}
}

func TestEnterSendsMessage(t *testing.T) {
	m := testModel()

	m.input.SetValue("  hello world  ")

	var cmd tea.Cmd
	m, cmd = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if cmd != nil {
		t.Fatal("enter should return no command")
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "message" || msgs[0].Text != "hello world" {
		t.Fatalf("sent = %+v, want trimmed message", msgs)
	}

	if m.input.Value() != "" {
		t.Error("input not reset after send")
	}
}

func TestEnterSkipsEmpty(t *testing.T) {
	m := testModel()

	m.input.SetValue("   ")

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, want no message", msgs)
	}
}

func TestTypingThrottle(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})

	if msgs := drainSend(t, m); len(msgs) != 1 || msgs[0].Type != "typing" {
		t.Fatalf("after first key: sent = %+v, want typing", msgs)
	}

	// A second key within the throttle window sends nothing.
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("after second key: sent = %+v, want nothing", msgs)
	}

	// Past the window, typing is sent again.
	m.lastTypingSent = time.Now().Add(-3 * time.Second)

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'!'}})

	if msgs := drainSend(t, m); len(msgs) != 1 || msgs[0].Type != "typing" {
		t.Fatalf("after throttle expiry: sent = %+v, want typing", msgs)
	}
}

func TestInputHistoryNavigation(t *testing.T) {
	m := testModel()
	m.history = []string{"first", "second"}
	m.historyIndex = 2

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyUp})

	if got := m.input.Value(); got != "second" {
		t.Errorf("first up = %q, want second", got)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyUp})

	if got := m.input.Value(); got != "first" {
		t.Errorf("second up = %q, want first", got)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})

	if got := m.input.Value(); got != "second" {
		t.Errorf("down = %q, want second", got)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})

	if got := m.input.Value(); got != "" {
		t.Errorf("down past end = %q, want empty", got)
	}
}

func TestIncomingMessageAppended(t *testing.T) {
	m := testModel()

	m, cmd := update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Color: "#ff0000", Text: "hello"}))

	if cmd == nil {
		t.Fatal("expected a follow-up command after incoming message")
	}

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	if !strings.Contains(m.messages[0].rendered, "bob") || !strings.Contains(m.messages[0].rendered, "hello") {
		t.Errorf("rendered = %q, want bob: hello", m.messages[0].rendered)
	}
}

func TestIncomingSystemAppended(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "system", Text: "bob joined the room"}))

	if len(m.messages) != 1 || !strings.Contains(m.messages[0].rendered, "[system]") {
		t.Errorf("messages = %+v, want [system] line", m.messages)
	}
}

func TestIncomingUsersList(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{
		Type: "users_list",
		Users: []UserInfo{
			{Nick: "alice", Color: "#fff", IsHost: true},
			{Nick: "bob", Color: "#000"},
		},
	}))

	if len(m.users) != 2 || !m.users[0].IsHost {
		t.Errorf("users = %+v", m.users)
	}
}

func TestIncomingHistoryReplay(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{
		Type: "history",
		Messages: []Message{
			{Type: "message", Nick: "old", Text: "one"},
			{Type: "message", Nick: "old", Text: "two"},
		},
	}))

	if len(m.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(m.messages))
	}
}

func TestMentionDetection(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Text: "hey @alice!"}))

	if len(m.messages) != 1 {
		t.Fatal("mention message not rendered")
	}

	// Mention styling applies an ANSI background (48); plain nicks only
	// use foreground color (38).
	if !strings.Contains(m.messages[0].rendered, "48;5;255") {
		t.Errorf("mention line has no background styling: %q", m.messages[0].rendered)
	}
}

func TestMentionCaseInsensitive(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Text: "hey @ALICE"}))

	if len(m.messages) != 1 || !strings.Contains(m.messages[0].rendered, "48;5;255") {
		t.Errorf("case-insensitive mention not styled: %q", m.messages[0].rendered)
	}
}

func TestNoMentionNoBellStyling(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Text: "plain text"}))

	if len(m.messages) != 1 {
		t.Fatal("plain message not rendered")
	}

	if strings.Contains(m.messages[0].rendered, "48;5;255") {
		t.Errorf("plain message has mention background: %q", m.messages[0].rendered)
	}
}

func TestWindowResize(t *testing.T) {
	m := testModel()

	// Wide terminal: sidebar shown, no compact mode.
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if !m.showSidebar {
		t.Error("wide terminal should show sidebar")
	}

	if m.compactMode {
		t.Error("wide terminal should not be in compact mode")
	}

	if m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		t.Errorf("viewport = %dx%d, want positive", m.viewport.Width, m.viewport.Height)
	}

	// Narrow terminal: no sidebar, compact mode.
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 60, Height: 30})

	if m.showSidebar {
		t.Error("narrow terminal should hide sidebar")
	}

	if !m.compactMode {
		t.Error("narrow terminal should be compact")
	}
}

func TestRelativeTime(t *testing.T) {
	now := time.Now()

	if got := relativeTime(now.Add(-30 * time.Second).Unix()); got != "30s" {
		t.Errorf("30s ago = %q", got)
	}

	if got := relativeTime(now.Add(-5 * time.Minute).Unix()); got != "5m" {
		t.Errorf("5m ago = %q", got)
	}

	if got := relativeTime(now.Add(-3 * time.Hour).Unix()); got != "3h" {
		t.Errorf("3h ago = %q", got)
	}
}

func TestQuitClearsTerminal(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/quit")

	if !handled || !quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}
}

func TestCommandReply(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/reply 3 hello world")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 {
		t.Fatalf("sent = %+v, want single reply message", msgs)
	}

	if msgs[0].Type != "message" || msgs[0].Text != "hello world" || msgs[0].ReplyToID != 3 {
		t.Fatalf("sent = %+v, want message replying to 3", msgs)
	}
}

func TestCommandReplyMissingArgs(t *testing.T) {
	m := testModel()

	for _, input := range []string{"/reply", "/reply 3", "/reply abc hi", "/reply 3   "} {
		handled, _ := handleCommand(&m, input)

		if !handled {
			t.Fatalf("%q not handled", input)
		}

		if msgs := drainSend(t, m); len(msgs) != 0 {
			t.Fatalf("%q sent = %+v, want no message", input, msgs)
		}
	}
}

func TestCommandReact(t *testing.T) {
	m := testModel()

	handled, quit := handleCommand(&m, "/react 3 +1")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "reaction" || msgs[0].ID != 3 || msgs[0].Text != "+1" {
		t.Fatalf("sent = %+v, want reaction +1 on 3", msgs)
	}
}

func TestCommandReactInvalid(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/react 3 bogus")

	if !handled {
		t.Fatal("expected handled")
	}

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, want no message", msgs)
	}

	found := false

	for _, line := range m.messages {
		if strings.Contains(line.rendered, "Invalid reaction") {
			found = true
		}
	}

	if !found {
		t.Error("no invalid-reaction feedback shown")
	}
}

func TestMessageIDDisplay(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", ID: 7, Nick: "bob", Text: "hello"}))

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	if !strings.Contains(m.messages[0].rendered, "#7") {
		t.Errorf("rendered = %q, want #7 prefix", m.messages[0].rendered)
	}

	if m.msgIndex[7] != 0 {
		t.Errorf("msgIndex[7] = %d, want 0", m.msgIndex[7])
	}
}

func TestReplyQuoteRendering(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m, _ = update(t, m, IncomingMessage(Message{
		Type:        "message",
		ID:          9,
		Nick:        "alice",
		Text:        "agreed",
		ReplyToID:   7,
		ReplyToNick: "bob",
		ReplyToText: "hello world",
	}))

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	rendered := m.messages[0].rendered

	if !strings.Contains(rendered, "> bob: hello world") {
		t.Errorf("rendered = %q, want quote line", rendered)
	}

	if !strings.Contains(rendered, "agreed") {
		t.Errorf("rendered = %q, want reply text", rendered)
	}
}

func TestReactionRendering(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{
		Type: "message",
		ID:   7,
		Nick: "bob",
		Text: "hello",
		Reactions: []Reaction{
			{Name: "+1", Count: 2},
			{Name: "laugh", Count: 1},
		},
	}))

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	if !strings.Contains(m.messages[0].rendered, "[+1 x2, laugh]") {
		t.Errorf("rendered = %q, want reaction suffix", m.messages[0].rendered)
	}
}

func TestReactionUpdateRerenders(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", ID: 7, Nick: "bob", Text: "hello"}))

	m, _ = update(t, m, IncomingMessage(Message{
		Type:      "reaction",
		ID:        7,
		Reactions: []Reaction{{Name: "+1", Count: 2}},
	}))

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	if !strings.Contains(m.messages[0].rendered, "[+1 x2]") {
		t.Errorf("rendered = %q, want reaction suffix after update", m.messages[0].rendered)
	}
}

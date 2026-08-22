package main

import (
	"strings"
	"testing"
	"time"

	"termchat/shared"

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
	}, "alice", "TEST", buildTheme("dark"))
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

func TestMentionStylingFollowsTheme(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Text: "hey @alice!"}))

	// Dark mention span: bold, black fg on white bg.
	if !strings.Contains(m.messages[0].rendered, "1;30;48;5;255") {
		t.Fatalf("dark mention styling missing: %q", m.messages[0].rendered)
	}

	m.theme = buildTheme("light")
	rerenderAll(&m)

	// Light mention span: bold, bright-white fg on dark slate bg.
	if !strings.Contains(m.messages[0].rendered, "1;97;48;5;235") {
		t.Errorf("light mention styling missing: %q", m.messages[0].rendered)
	}

	if strings.Contains(m.messages[0].rendered, "1;30;48;5;255") {
		t.Errorf("dark mention styling survived theme switch: %q", m.messages[0].rendered)
	}
}

func TestCmdThemeSwitchRerenders(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = update(t, m, IncomingMessage(Message{Type: "message", ID: 1, Nick: "bob", Text: "hey @alice!"}))
	m, _ = update(t, m, IncomingMessage(Message{Type: "system", Text: "bob joined"}))

	handled, quit := handleCommand(&m, "/theme light")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	if m.theme.Name != "light" {
		t.Fatalf("theme = %q, want light", m.theme.Name)
	}

	view := m.View()

	// Light markers: mention span and the dim system line.
	if !strings.Contains(view, "1;97;48;5;235") || !strings.Contains(view, "38;5;242") {
		t.Error("view missing light styling")
	}

	// Dark-only colors: dim fg 8 and the dark mention span.
	if strings.Contains(view, ";5;8m") || strings.Contains(view, "1;30;48;5;255") {
		t.Error("view kept dark palette colors")
	}

	found := false

	for _, line := range m.messages {
		if strings.Contains(line.rendered, "Theme set to light") {
			found = true
		}
	}

	if !found {
		t.Error("no confirmation feedback shown")
	}
}

func TestCmdThemeNoArgLists(t *testing.T) {
	m := testModel()

	handleCommand(&m, "/theme")

	out := ""

	for _, line := range m.messages {
		out += line.rendered
	}

	for _, name := range themeNames {
		if !strings.Contains(out, name) {
			t.Errorf("listing missing %q: %q", name, out)
		}
	}

	if !strings.Contains(out, "* dark") {
		t.Errorf("active theme not marked: %q", out)
	}
}

func TestCmdThemeUnknownKeepsTheme(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{Type: "message", Nick: "bob", Text: "hey @alice!"}))
	before := m.messages[0].rendered

	handled, quit := handleCommand(&m, "/theme nope")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	if m.theme.Name != "dark" {
		t.Errorf("theme changed to %q on invalid input", m.theme.Name)
	}

	if m.messages[0].rendered != before {
		t.Error("lines re-rendered despite rejected theme")
	}

	found := false

	for _, line := range m.messages {
		if strings.Contains(line.rendered, "unknown theme") && strings.Contains(line.rendered, "system") {
			found = true
		}
	}

	if !found {
		t.Error("no unknown-theme feedback shown")
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

	if !strings.Contains(m.messages[0].rendered, "[\U0001f44d x2, \U0001f606]") {
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

	if !strings.Contains(m.messages[0].rendered, "[\U0001f44d x2]") {
		t.Errorf("rendered = %q, want reaction suffix after update", m.messages[0].rendered)
	}
}

func TestUnknownReactionFallsBackToName(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, IncomingMessage(Message{
		Type:      "message",
		ID:        7,
		Nick:      "bob",
		Text:      "hello",
		Reactions: []Reaction{{Name: "mystery", Count: 1}},
	}))

	if !strings.Contains(m.messages[0].rendered, "[mystery]") {
		t.Errorf("rendered = %q, want raw name for unknown reaction", m.messages[0].rendered)
	}
}

func TestCommandRegistryIntegrity(t *testing.T) {
	seen := map[string]bool{}

	for _, c := range commands {
		if seen[c.name] {
			t.Errorf("duplicate command %q", c.name)
		}
		seen[c.name] = true

		if c.handler == nil {
			t.Errorf("%q has no handler", c.name)
		}

		if !strings.HasPrefix(c.usage, c.name) {
			t.Errorf("usage %q does not start with name %q", c.usage, c.name)
		}

		if strings.TrimSpace(c.description) == "" {
			t.Errorf("%q has no description", c.name)
		}
	}

	if len(commands) < 8 {
		t.Errorf("registry = %d commands, want at least 8", len(commands))
	}
}

func TestFilterCommands(t *testing.T) {
	all := filterCommands("/")

	if len(all) != len(commands) {
		t.Fatalf("filter / = %d matches, want %d", len(all), len(commands))
	}

	matches := filterCommands("/RE")

	if len(matches) != 2 || matches[0].name != "/reply" || matches[1].name != "/react" {
		t.Fatalf("filter /RE = %+v, want /reply then /react", commandNames(matches))
	}

	if got := filterCommands("/zzz"); len(got) != 0 {
		t.Fatalf("filter /zzz = %+v, want no matches", commandNames(got))
	}
}

func commandNames(cmds []command) []string {
	var out []string

	for _, c := range cmds {
		out = append(out, c.name)
	}

	return out
}

func TestHelpListsAllCommands(t *testing.T) {
	m := testModel()

	handled, _ := handleCommand(&m, "/help")

	if !handled {
		t.Fatal("expected handled")
	}

	if len(m.messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(m.messages))
	}

	for _, c := range commands {
		if !strings.Contains(m.messages[0].rendered, c.usage) ||
			!strings.Contains(m.messages[0].rendered, c.description) {
			t.Errorf("help text missing %q: %q", c.usage, m.messages[0].rendered)
		}
	}
}

func typeRunes(t *testing.T, m Model, runes string) Model {
	t.Helper()

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(runes)})

	return m
}

func TestCompletionAutoOpenOnSlash(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/")

	if !m.showPopup {
		t.Fatal("popup should open when input becomes /")
	}

	matches := completionMatches(&m)

	if len(matches) != len(commands) {
		t.Fatalf("matches = %d, want all %d", len(matches), len(commands))
	}

	m = typeRunes(t, m, "nic")

	matches = completionMatches(&m)

	if len(matches) != 1 || matches[0].insert != "/nick " {
		t.Fatalf("matches = %+v, want only /nick", matches)
	}
}

func TestCompletionClosesOnNoMatchAndSpace(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/zz")

	if m.showPopup {
		t.Error("popup should close when nothing matches")
	}

	m = testModel()

	m = typeRunes(t, m, "/nic")

	if !m.showPopup {
		t.Fatal("popup should be open for /nic")
	}

	m = typeRunes(t, m, " ")

	if m.showPopup {
		t.Error("popup should close once an argument is typed")
	}
}

func TestCompletionTabOpensAndAccepts(t *testing.T) {
	m := testModel()

	m.input.SetValue("/col")

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if !m.showPopup {
		t.Fatal("tab should open the popup for a slash prefix")
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if m.showPopup {
		t.Error("second tab should accept and close the popup")
	}

	if got := m.input.Value(); got != "/color " {
		t.Errorf("input = %q, want /color with trailing space", got)
	}
}

func TestCompletionEnterInsertsWithoutSending(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/re")

	matches := completionMatches(&m)

	if len(matches) != 2 || matches[0].insert != "/reply " {
		t.Fatalf("matches = %+v, want /reply first", matches)
	}

	drainSend(t, m) // discard the typing frame from the keystrokes

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, accept must not send anything", msgs)
	}

	if m.showPopup {
		t.Error("popup should close after accepting")
	}

	if got := m.input.Value(); got != "/reply " {
		t.Fatalf("input = %q, want inserted /reply with trailing space", got)
	}

	// Finishing the arguments and pressing enter dispatches the command.
	m = typeRunes(t, m, "5 hi")

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].ReplyToID != 5 || msgs[0].Text != "hi" {
		t.Fatalf("sent = %+v, want reply to 5", msgs)
	}
}

func TestCompletionUpDownNavigation(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/")

	down := tea.KeyMsg{Type: tea.KeyDown}
	up := tea.KeyMsg{Type: tea.KeyUp}

	m, _ = update(t, m, down)
	m, _ = update(t, m, down)

	if m.selected != 2 {
		t.Fatalf("after two downs selected = %d, want 2", m.selected)
	}

	m, _ = update(t, m, up)

	if m.selected != 1 {
		t.Fatalf("after up selected = %d, want 1", m.selected)
	}

	for i := 0; i < 5; i++ {
		m, _ = update(t, m, up)
	}

	if m.selected != 0 {
		t.Fatalf("selected = %d, want clamped at 0", m.selected)
	}

	for i := 0; i < 20; i++ {
		m, _ = update(t, m, down)
	}

	if m.selected != len(commands)-1 {
		t.Fatalf("selected = %d, want clamped at %d", m.selected, len(commands)-1)
	}
}

func TestCompletionEscDismisses(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/n")

	if !m.showPopup {
		t.Fatal("popup should be open")
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEscape})

	if m.showPopup {
		t.Error("esc should dismiss the popup")
	}

	// Typing again reopens and refilters.
	m = typeRunes(t, m, "i")

	matches := completionMatches(&m)

	if !m.showPopup || len(matches) != 1 || matches[0].insert != "/nick " {
		t.Fatalf(
			"matches = %+v open = %v, want reopened on /nick",
			matches,
			m.showPopup,
		)
	}
}

func TestCompletionShrinksViewport(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	closed := m.viewport.Height

	m = typeRunes(t, m, "/")

	open := m.viewport.Height
	want := len(filterCommands("/")) + 2 // rounded border

	if closed-open != want {
		t.Errorf("viewport shrank by %d rows, want %d", closed-open, want)
	}

	if open <= 0 {
		t.Errorf("viewport height = %d while popup is open, want positive", open)
	}
}

func TestViewRendersCompletionPopup(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if strings.Contains(m.View(), "/password [pass]") {
		t.Error("closed popup rendered in view")
	}

	m = typeRunes(t, m, "/pass")

	view := m.View()

	if !strings.Contains(view, "/password [pass]") ||
		!strings.Contains(view, "set or remove room password") {
		t.Error("open popup not rendered with usage and description")
	}
}

func TestEmojiTableIntegrity(t *testing.T) {
	seen := map[string]bool{}

	for _, e := range emojis {
		if seen[e.name] {
			t.Errorf("duplicate emoji shortcode %q", e.name)
		}
		seen[e.name] = true

		if e.glyph == "" {
			t.Errorf("shortcode %q has no glyph", e.name)
		}
	}

	for _, name := range shared.ReactionNames {
		if !seen[name] {
			t.Errorf("reaction name %q missing from the emoji table", name)
		}
	}
}

func TestEmojiAutoOpenMidText(t *testing.T) {
	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	closed := m.viewport.Height

	m = typeRunes(t, m, "hi :")

	if !m.showPopup {
		t.Fatal("popup should open on a bare colon mid-text")
	}

	matches := completionMatches(&m)

	if len(matches) != len(emojis) {
		t.Fatalf("matches = %d, want all %d", len(matches), len(emojis))
	}

	if got := m.viewport.Height; closed-got != len(emojis)+2 {
		t.Errorf("viewport shrank by %d rows, want %d", closed-got, len(emojis)+2)
	}

	m = typeRunes(t, m, "fi")

	matches = completionMatches(&m)

	if len(matches) != 1 || matches[0].primary != ":fire:" || matches[0].insert != "\U0001f525 " {
		t.Fatalf("matches = %+v, want only :fire:", matches)
	}
}

func TestEmojiAcceptPreservesPrefix(t *testing.T) {
	m := testModel()

	// SetValue bypasses the refresh, so the popup starts closed.
	m.input.SetValue("hi :hear")

	drainSend(t, m) // discard typing frames as we go

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if !m.showPopup {
		t.Fatal("tab should open the popup for :hear")
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if m.showPopup {
		t.Error("second tab should accept and close the popup")
	}

	want := "hi \u2764\ufe0f "

	if got := m.input.Value(); got != want {
		t.Errorf("input = %q, want %q", got, want)
	}

	if msgs := drainSend(t, m); len(msgs) != 0 {
		t.Fatalf("sent = %+v, accept must not send anything", msgs)
	}
}

func TestEmojiSecondColonCloses(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "hi :fire:")

	if m.showPopup {
		t.Error("popup should close once the shortcode is terminated")
	}

	matches := completionMatches(&m)

	if len(matches) != 0 {
		t.Errorf("matches = %+v, want none", matches)
	}
}

func TestReactArgSuggestions(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/react 7 f")

	if !m.showPopup {
		t.Fatal("popup should open for a /react name argument")
	}

	matches := completionMatches(&m)

	if len(matches) != 1 || matches[0].primary != ":fire:" || matches[0].insert != "fire" {
		t.Fatalf("matches = %+v, want only :fire: inserting fire", matches)
	}

	// Trailing space with a valid id lists every reaction.
	m = testModel()
	m = typeRunes(t, m, "/react 7 ")

	matches = completionMatches(&m)

	if len(matches) != len(shared.ReactionNames) {
		t.Fatalf("matches = %d, want all %d", len(matches), len(shared.ReactionNames))
	}

	// A non-numeric id or a still-unfinished id suggests nothing.
	for _, bad := range []string{"/react x f", "/react 7"} {
		m := testModel()
		m = typeRunes(t, m, bad)

		if m.showPopup || len(completionMatches(&m)) != 0 {
			t.Errorf("%q opened the react popup, want closed", bad)
		}
	}
}

func TestReactAcceptCompletesNameAndSends(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "/react 7 f")

	drainSend(t, m) // discard the typing frame

	// The popup is already open from typing; tab accepts in place.
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyTab})

	if m.showPopup {
		t.Fatal("popup should close after accepting")
	}

	if got := m.input.Value(); got != "/react 7 fire" {
		t.Fatalf("input = %q, want completed /react 7 fire", got)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	msgs := drainSend(t, m)

	if len(msgs) != 1 || msgs[0].Type != "reaction" || msgs[0].ID != 7 || msgs[0].Text != "fire" {
		t.Fatalf("sent = %+v, want reaction 7 fire", msgs)
	}
}

func TestNoCommandSuggestionsMidText(t *testing.T) {
	m := testModel()

	m = typeRunes(t, m, "hello /ni")

	if m.showPopup {
		t.Error("commands should not be suggested mid-message")
	}
}

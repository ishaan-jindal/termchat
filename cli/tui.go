package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

var (
	systemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	mentionStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("255")).
			Foreground(lipgloss.Color("0")).
			Bold(true)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("250")).
			Background(lipgloss.Color("236")).
			Padding(0, 1)

	completionSelectedStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("238")).
				Bold(true)

	usersHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("238")).
				Padding(0, 1)
)

type IncomingMessage Message

// chatLine holds one rendered chat message; the raw Message is kept so the
// line can be re-rendered when its reactions change.
type chatLine struct {
	msg      Message
	rendered string
}

type Model struct {
	conn *Connection

	messages []chatLine
	input    textarea.Model

	// msgIndex maps a message ID to its index in messages.
	msgIndex map[int64]int

	nick      string
	room      string
	users     []UserInfo
	connected bool

	IsHost   bool
	HostIP   string
	HostPort int

	viewport viewport.Model
	width    int
	height   int

	autoScroll bool

	compactMode bool
	showSidebar bool

	history      []string
	historyIndex int

	lastTypingSent time.Time

	// usersRequested makes the next users_list print into the chat log.
	usersRequested bool

	// clockOffset is server_time minus local time from the latest
	// users_list; relative times are rendered on the server's timeline.
	clockOffset int64

	showPopup bool
	selected  int
}

func NewModel(conn *Connection, nick string, room string) Model {
	ti := textarea.New()

	ti.Placeholder = "Type a message..."
	ti.Focus()

	ti.ShowLineNumbers = false
	ti.SetHeight(3)
	ti.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(0, 0)

	return Model{
		conn:         conn,
		messages:     []chatLine{},
		msgIndex:     map[int64]int{},
		input:        ti,
		nick:         nick,
		room:         room,
		users:        []UserInfo{},
		connected:    true,
		viewport:     vp,
		history:      []string{},
		historyIndex: 0,
		autoScroll:   true,
	}
}

func (m Model) Init() tea.Cmd {
	return waitForMessage(m.conn)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		switch msg.String() {

		case "ctrl+c":
			clearTerminal()
			return m, tea.Quit

		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.autoScroll = m.viewport.AtBottom()
			return m, cmd

		case "tab":
			if m.showPopup {
				acceptCompletion(&m)
				return m, nil
			}

			m.showPopup = true
			refreshCompletion(&m)

			return m, nil

		case "esc":
			dismissCompletion(&m)

			return m, nil

		case "up":
			if m.showPopup {
				m.selected = max(m.selected-1, 0)
				return m, nil
			}

			if m.input.Line() == 0 {
				if len(m.history) > 0 && m.historyIndex > 0 {
					m.historyIndex--
					m.input.SetValue(m.history[m.historyIndex])
				}
				return m, nil
			}

		case "down":
			if m.showPopup {
				if n := len(completionMatches(&m)); n > 0 {
					m.selected = min(m.selected+1, n-1)
				}
				return m, nil
			}

			totalLines := strings.Count(m.input.Value(), "\n") + 1
			if m.input.Line() >= totalLines-1 {
				if len(m.history) > 0 && m.historyIndex < len(m.history)-1 {
					m.historyIndex++
					m.input.SetValue(m.history[m.historyIndex])
				} else {
					m.historyIndex = len(m.history)
					m.input.SetValue("")
				}
				return m, nil
			}

		case "alt+enter":
			m.input.InsertRune('\n')
			refreshCompletion(&m)
			return m, nil

		case "enter":
			if m.showPopup {
				acceptCompletion(&m)
				return m, nil
			}

			text := strings.TrimSpace(m.input.Value())
			if strings.HasPrefix(text, "/") {
				handled, quit := handleCommand(&m, text)
				if handled {
					m.input.Reset()
					if quit {
						return m, tea.Quit
					}
					return m, nil
				}
			}
			if text != "" {
				m.history = append(m.history, text)
				m.historyIndex = len(m.history)

				trySend(&m, Message{
					Type: "message",
					Text: text,
				})
				m.input.Reset()
			}
			return m, nil
		}

		var cmd tea.Cmd

		previousValue := m.input.Value()
		m.input, cmd = m.input.Update(msg)

		if m.input.Value() != previousValue {
			refreshCompletion(&m)

			if time.Since(m.lastTypingSent) > 2*time.Second {
				trySend(&m, Message{Type: "typing"})
				m.lastTypingSent = time.Now()
			}
		}

		return m, cmd

	case tea.MouseMsg:
		switch msg.Button {

		case tea.MouseButtonWheelUp:
			m.viewport.ScrollUp(3)

		case tea.MouseButtonWheelDown:
			m.viewport.ScrollDown(3)
		}

		m.autoScroll = m.viewport.AtBottom()

		return m, nil

	case IncomingMessage:

		switch msg.Type {

		case "system", "message":
			appendFormattedMessage(&m, Message(msg))

		case "reaction":
			idx, ok := m.msgIndex[msg.ID]
			if ok {
				target := m.messages[idx].msg

				// A growing vote total means a reaction was added,
				// not removed.
				before, after := 0, 0
				for _, r := range target.Reactions {
					before += r.Count
				}
				for _, r := range msg.Reactions {
					after += r.Count
				}

				if target.Nick == m.nick && msg.Nick != "" && msg.Nick != m.nick && after > before {
					print("\a")
					notify(fmt.Sprintf("%s reacted to your message", msg.Nick), target.Text)
				}

				m.messages[idx].msg.Reactions = msg.Reactions
				m.messages[idx].rendered = renderMessage(&m, m.messages[idx].msg)
			}

		case "users_list":
			m.users = msg.Users

			if msg.ServerTime != 0 {
				m.clockOffset = msg.ServerTime - time.Now().Unix()
			}

			if m.usersRequested {
				appendUsersList(&m)
				m.usersRequested = false
			}

		case "history":
			for _, historyMsg := range msg.Messages {
				appendFormattedMessage(&m, historyMsg)
			}
		}

		wasAtBottom := m.autoScroll || m.viewport.AtBottom()

		m.viewport.SetContent(strings.Join(renderedLines(&m), "\n"))

		if wasAtBottom {
			m.viewport.GotoBottom()
		}

		return m, waitForMessage(m.conn)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		m.compactMode = msg.Width < 100
		m.showSidebar = msg.Width >= 70

		sidebarWidth := 0

		if m.showSidebar {
			if m.compactMode {
				sidebarWidth = 16
			} else {
				sidebarWidth = 22
			}
		}

		m.viewport.Width = max(msg.Width-sidebarWidth-10, 20)

		inputWidth := max(m.width-14, 20)
		m.input.SetWidth(inputWidth)

		resizeViewport(&m)

		m.viewport.SetContent(strings.Join(renderedLines(&m), "\n"))

		return m, nil
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)
	m.input.SetHeight(textareaHeight(m.input))

	return m, cmd
}

func (m Model) View() string {
	scrollInfo := ""

	if !m.viewport.AtTop() {
		scrollInfo += " ^"
	}

	if !m.viewport.AtBottom() {
		scrollInfo += " v"
	}

	messagesPanel := panelStyle.
		Width(m.viewport.Width + 4).
		Height(m.viewport.Height).
		Render(m.viewport.View())

	var content string

	if m.showSidebar {
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			messagesPanel,
			renderUsers(m),
		)
	} else {
		content = messagesPanel
	}

	input := panelStyle.
		Width(m.width - 6).
		Render(m.input.View())

	statusText := fmt.Sprintf(
		"Connected - Room %s - %d users%s",
		m.room,
		len(m.users),
		scrollInfo,
	)

	if m.IsHost {
		statusText = fmt.Sprintf(
			"SELF-HOSTED - Room %s - %s:%d - %d users%s",
			m.room,
			m.HostIP,
			m.HostPort,
			len(m.users),
			scrollInfo,
		)
	}

	status := panelStyle.
		Width(m.width - 6).
		Render(
			statusStyle.Render(
				statusText,
			),
		)

	rows := []string{content}

	if m.showPopup {
		if popup := renderCompletion(m); popup != "" {
			rows = append(rows, popup)
		}
	}

	ui := lipgloss.JoinVertical(
		lipgloss.Left,
		append(rows, input, status)...,
	)

	return ui
}

func renderUsers(m Model) string {
	var lines []string

	width := 20
	if m.compactMode {
		width = 14
	}

	header := usersHeaderStyle.Width(width - 2).Align(lipgloss.Center).Render("USERS")
	lines = append(lines, header)
	lines = append(lines, strings.Repeat("-", width-2))
	lines = append(lines, "")

	for _, user := range m.users {
		nick := user.Nick
		if m.compactMode && len(nick) > 8 {
			nick = nick[:8]
		}

		joined := relativeTime(user.JoinedAt - m.clockOffset)

		status := ""
		if user.IsHost {
			status += "[host] "
		}
		if user.Typing {
			status += "[...] "
		}

		coloredNick := lipgloss.NewStyle().
			Foreground(lipgloss.Color(user.Color)).
			Bold(true).
			Render("* " + nick)

		line := fmt.Sprintf(
			"%-12s%4s %s",
			coloredNick,
			joined,
			status,
		)

		lines = append(lines, line)
	}

	content := strings.Join(lines, "\n")
	return panelStyle.
		Width(width).
		Height(m.viewport.Height).
		Render(content)
}

func appendFormattedMessage(m *Model, msg Message) {
	switch msg.Type {

	case "system":
		m.messages = append(m.messages, chatLine{
			msg:      msg,
			rendered: renderSystemMessage(m, msg),
		})

	case "message":
		m.messages = append(m.messages, chatLine{
			msg:      msg,
			rendered: renderMessage(m, msg),
		})

		title := ""

		switch {
		case isMention(msg, m.nick):
			title = fmt.Sprintf("%s mentioned you", msg.Nick)

		case msg.ReplyToID != 0 && msg.ReplyToNick == m.nick && msg.Nick != m.nick:
			title = fmt.Sprintf("%s replied to your message", msg.Nick)
		}

		if title != "" {
			print("\a")
			notify(title, msg.Text)
		}

		if msg.ID != 0 {
			m.msgIndex[msg.ID] = len(m.messages) - 1
		}
	}
}

func renderedLines(m *Model) []string {
	lines := make([]string, 0, len(m.messages))

	for _, line := range m.messages {
		lines = append(lines, line.rendered)
	}

	return lines
}

func isMention(msg Message, nick string) bool {
	return strings.Contains(
		strings.ToLower(msg.Text),
		"@"+strings.ToLower(nick),
	)
}

func renderSystemMessage(m *Model, msg Message) string {
	plain := "[system] " + msg.Text

	if m.viewport.Width > 0 && runtime.GOARCH != "386" {
		plain = wordwrap.String(plain, m.viewport.Width)
	}

	return systemStyle.Render(plain)
}

func renderMessage(m *Model, msg Message) string {
	mentioned := isMention(msg, m.nick)

	idPrefix := ""
	if msg.ID != 0 {
		idPrefix = fmt.Sprintf("#%d ", msg.ID)
	}

	nickStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.Color)).
		Bold(true)

	prefix := idPrefix + msg.Nick + ": "
	availableWidth := max(m.viewport.Width-len(prefix), 10)
	wrapped := msg.Text
	if runtime.GOARCH != "386" {
		wrapped = wordwrap.String(msg.Text, availableWidth)
	}
	lines := strings.Split(wrapped, "\n")

	if mentioned {
		nickStyle = nickStyle.Background(mentionStyle.GetBackground())
		for i := range lines {
			if i == 0 {
				lines[i] = systemStyle.Render(idPrefix) + nickStyle.Render(msg.Nick) + mentionStyle.Render(": "+lines[i])
			} else {
				lines[i] = mentionStyle.Render(strings.Repeat(" ", len(prefix)) + lines[i])
			}
		}
	} else {
		renderedNick := nickStyle.Render(msg.Nick)
		for i := range lines {
			if i == 0 {
				lines[i] = systemStyle.Render(idPrefix) + renderedNick + ": " + lines[i]
			} else {
				lines[i] = strings.Repeat(" ", len(prefix)) + lines[i]
			}
		}
	}

	var out []string

	if quote := formatQuote(msg, availableWidth); quote != "" {
		out = append(out, quote)
	}

	out = append(out, lines...)

	if len(msg.Reactions) > 0 {
		last := len(out) - 1
		out[last] += systemStyle.Render("  " + formatReactions(msg.Reactions))
	}

	return strings.Join(out, "\n")
}

// formatQuote renders the quoted message of a reply as a dim quote line, or
// an empty string when the message is not a reply.
func formatQuote(msg Message, width int) string {
	if msg.ReplyToID == 0 {
		return ""
	}

	quote := "> " + msg.ReplyToNick + ": " + msg.ReplyToText

	if width > 0 && runtime.GOARCH != "386" {
		quote = wordwrap.String(quote, width)
	}

	return systemStyle.Render(quote)
}

// formatReactions renders reaction counts as emoji glyphs instead of raw
// names, with a leading blank line separator.
func formatReactions(reactions []Reaction) string {
	if len(reactions) == 0 {
		return ""
	}

	parts := make([]string, 0, len(reactions))

	for _, r := range reactions {
		glyph := emojiGlyph(r.Name)

		if glyph == "" {
			glyph = r.Name
		}

		if r.Count > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", glyph, r.Count))
		} else {
			parts = append(parts, glyph)
		}
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

// appendUsersList writes the current room roster into the chat log as system
// lines, marking the host.
func appendUsersList(m *Model) {
	appendFormattedMessage(m, Message{
		Type: "system",
		Text: fmt.Sprintf("Users (%d):", len(m.users)),
	})

	for _, user := range m.users {
		line := "  " + user.Nick

		if user.IsHost {
			line += " (host)"
		}

		appendFormattedMessage(m, Message{
			Type: "system",
			Text: line,
		})
	}
}

// completionMatches returns the suggestions for the current input, or nil
// when the popup is closed or nothing matches.
func completionMatches(m *Model) []suggestion {
	if !m.showPopup {
		return nil
	}

	matches, _ := matchSuggestions(m.input.Value())

	return matches
}

// refreshCompletion reopens or refilters the popup from the current input.
func refreshCompletion(m *Model) {
	matches, _ := matchSuggestions(m.input.Value())

	open := len(matches) > 0

	if open && m.selected >= len(matches) {
		m.selected = len(matches) - 1
	}

	if !open {
		m.selected = 0
	}

	m.showPopup = open

	resizeViewport(m)
}

func dismissCompletion(m *Model) {
	if !m.showPopup {
		return
	}

	m.showPopup = false
	m.selected = 0

	resizeViewport(m)
}

// acceptCompletion inserts the selected suggestion in place of the current
// token and closes the popup.
func acceptCompletion(m *Model) {
	matches, tokenLen := matchSuggestions(m.input.Value())

	m.showPopup = false

	if len(matches) == 0 {
		return
	}

	m.selected = min(m.selected, len(matches)-1)

	value := m.input.Value()
	m.input.SetValue(value[:len(value)-tokenLen] + matches[m.selected].insert)
	m.input.CursorEnd()

	resizeViewport(m)
}

// resizeViewport refits the viewport height from the terminal size, the
// input height and the popup height.
func resizeViewport(m *Model) {
	popupHeight := len(completionMatches(m))
	if popupHeight > 0 {
		popupHeight += 2 // rounded border
	}

	m.viewport.Height = max(
		m.height-textareaHeight(m.input)-popupHeight-7,
		5,
	)
}

func renderCompletion(m Model) string {
	matches := completionMatches(&m)
	if len(matches) == 0 {
		return ""
	}

	sel := min(m.selected, len(matches)-1)

	width := 0

	for _, s := range matches {
		width = max(width, len(s.primary))
	}

	rows := make([]string, 0, len(matches))

	for i, s := range matches {
		row := fmt.Sprintf(
			"%-*s  %s",
			width,
			s.primary,
			systemStyle.Render(s.detail),
		)

		if i == sel {
			row = completionSelectedStyle.Render(row)
		}

		rows = append(rows, row)
	}

	return panelStyle.
		Width(m.width - 6).
		Render(strings.Join(rows, "\n"))
}

func clearTerminal() {
	print("\033[H\033[2J")
}

func textareaHeight(input textarea.Model) int {
	lines := strings.Count(input.Value(), "\n") + 1
	if lines < 3 {
		return 3
	}
	if lines > 8 {
		return 8
	}
	return lines
}

func relativeTime(unix int64) string {
	d := time.Since(time.Unix(unix, 0))

	if d <= 0 {
		return "now"
	}

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	return fmt.Sprintf("%dh", int(d.Hours()))
}

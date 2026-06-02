package main

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"termchat/shared"

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

	usersHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("15")).
				Background(lipgloss.Color("238")).
				Padding(0, 1)
)

type IncomingMessage Message

type Model struct {
	conn *Connection

	messages []string
	input    textarea.Model

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
		messages:     []string{},
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

		case "up":
			if m.input.Line() == 0 {
				if len(m.history) > 0 && m.historyIndex > 0 {
					m.historyIndex--
					m.input.SetValue(m.history[m.historyIndex])
				}
				return m, nil
			}

		case "down":
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
			return m, nil

		case "enter":
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

				m.conn.conn.WriteJSON(Message{
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
			if time.Since(m.lastTypingSent) > 2*time.Second {
				m.conn.conn.WriteJSON(Message{
					Type: "typing",
				})
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

		case "users_list":
			m.users = msg.Users

		case "history":
			for _, historyMsg := range msg.Messages {
				appendFormattedMessage(&m, historyMsg)
			}
		}

		wasAtBottom := m.autoScroll || m.viewport.AtBottom()

		m.viewport.SetContent(strings.Join(m.messages, "\n"))

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

		m.viewport.Height = max(msg.Height-10, 5)

		inputHeight := textareaHeight(m.input)
		m.viewport.Height = max(
			msg.Height-inputHeight-7,
			5,
		)

		inputWidth := max(m.width-14, 20)
		m.input.SetWidth(inputWidth)

		m.viewport.SetContent(strings.Join(m.messages, "\n"))

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
		scrollInfo += " ↑"
	}

	if !m.viewport.AtBottom() {
		scrollInfo += " ↓"
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
		"Connected • Room %s • %d users%s",
		m.room,
		len(m.users),
		scrollInfo,
	)

	if m.IsHost {
		statusText = fmt.Sprintf(
			"SELF-HOSTED • Room %s • %s:%d • %d users%s",
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

	ui := lipgloss.JoinVertical(
		lipgloss.Left,
		content,
		input,
		status,
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
	lines = append(lines, strings.Repeat("─", width-2))
	lines = append(lines, "")

	for _, user := range m.users {
		nick := user.Nick
		if m.compactMode && len(nick) > 8 {
			nick = nick[:8]
		}

		joined := relativeTime(user.JoinedAt)

		status := ""
		if user.Typing {
			status = "✍ "
		}

		coloredNick := lipgloss.NewStyle().
			Foreground(lipgloss.Color(user.Color)).
			Bold(true).
			Render("● " + nick)

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
		plain := "[system] " + msg.Text

		if m.viewport.Width > 0 && runtime.GOARCH != "386" {
			plain = wordwrap.String(plain, m.viewport.Width)
		}

		formatted := systemStyle.Render(plain)

		m.messages = append(m.messages, formatted)

	case "message":
		mentioned := strings.Contains(
			strings.ToLower(msg.Text),
			"@"+strings.ToLower(m.nick),
		)
		nickStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(msg.Color)).
			Bold(true)

		prefix := msg.Nick + ": "
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
					lines[i] = nickStyle.Render(msg.Nick) + mentionStyle.Render(": "+lines[i])
				} else {
					lines[i] = mentionStyle.Render(strings.Repeat(" ", len(prefix)) + lines[i])
				}
			}
			print("\a")
		} else {
			renderedNick := nickStyle.Render(msg.Nick)
			for i := range lines {
				if i == 0 {
					lines[i] = renderedNick + ": " + lines[i]
				} else {
					lines[i] = strings.Repeat(" ", len(prefix)) + lines[i]
				}
			}
		}

		m.messages = append(m.messages, strings.Join(lines, "\n"))
	}
}

func handleCommand(m *Model, input string) (handled bool, quit bool) {
	parts := strings.Split(input, " ")

	cmd := parts[0]

	switch cmd {

	case "/clear":
		m.messages = []string{}
		m.viewport.SetContent("")
		return true, false

	case "/quit":
		clearTerminal()
		return true, true

	case "/help":
		m.messages = append(
			m.messages,
			systemStyle.Render(
				"Commands: /help /clear /nick /color /quit",
			),
		)

		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()

		return true, false

	case "/nick":
		if len(parts) < 2 {
			return true, false
		}

		newNick := parts[1]

		m.conn.conn.WriteJSON(Message{
			Type:    "nick",
			NewNick: newNick,
		})

		m.nick = newNick

		return true, false

	case "/color":
		if len(parts) < 2 {
			return true, false
		}

		color := parts[1]

		if !shared.IsValidHexColor(color) {
			m.messages = append(
				m.messages,
				systemStyle.Render("Invalid hex color"),
			)

			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()

			return true, false
		}

		m.conn.conn.WriteJSON(Message{
			Type:  "color",
			Color: color,
		})

		cfg := loadConfig()
		cfg.Color = color
		saveConfig(cfg)

		return true, false
	}

	return false, false
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

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	return fmt.Sprintf("%dh", int(d.Hours()))
}

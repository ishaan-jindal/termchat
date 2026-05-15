package main

import (
	"regexp"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"
)

var (
	borderStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	systemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	nickStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
)

type IncomingMessage Message

type Model struct {
	conn *Connection

	messages []string
	input    textinput.Model

	nick string
	room string

	viewport viewport.Model
	width    int
	height   int
}

func NewModel(conn *Connection, nick string, room string) Model {
	ti := textinput.New()

	ti.Placeholder = "Type a message..."
	ti.Focus()

	vp := viewport.New(0, 0)

	return Model{
		conn:     conn,
		messages: []string{},
		input:    ti,
		nick:     nick,
		room:     room,
		viewport: vp,
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

		case "up", "down", "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd

		case "enter":
			text := m.input.Value()

			if strings.HasPrefix(text, "/") {
				ok := handleCommand(&m, text)

				m.input.SetValue("")

				if !ok {
					return m, tea.Quit
				}

				return m, nil
			}

			if text != "" {
				m.conn.conn.WriteJSON(Message{
					Type: "message",
					Text: text,
				})

				m.input.SetValue("")
			}
		}

		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)

		return m, cmd

	case IncomingMessage:
		switch msg.Type {

		case "system":
			formatted := systemStyle.Render("[system] " + msg.Text)
			if m.viewport.Width > 0 {
				if runtime.GOARCH != "386" {
					formatted = wordwrap.String(formatted, m.viewport.Width)
				}
			}
			m.messages = append(m.messages, formatted)

		case "message":
			style := lipgloss.NewStyle().
				Foreground(lipgloss.Color(msg.Color)).
				Bold(true)
			nick := style.Render(msg.Nick)
			formatted := nick + ": " + msg.Text
			if m.viewport.Width > 0 {
				if runtime.GOARCH != "386" {
					formatted = wordwrap.String(formatted, m.viewport.Width)
				}
			}
			m.messages = append(m.messages, formatted)

		case "users_list":
			m.messages = append(
				m.messages,
				systemStyle.Render("Online: "+msg.Text),
			)
		}

		atBottom := m.viewport.AtBottom()

		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		if atBottom {
			m.viewport.GotoBottom()
		}

		return m, waitForMessage(m.conn)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3
		inputHeight := 3
		borderHeight := 2

		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height - headerHeight - inputHeight - borderHeight

		m.input.Width = msg.Width - 6

		m.viewport.SetContent(strings.Join(m.messages, "\n"))

		return m, nil
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

func (m Model) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Render("Room: " + m.room)

	content := m.viewport.View()

	input := "> " + m.input.View()

	ui := lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		content,
		"",
		input,
	)

	return borderStyle.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(ui)
}

func handleCommand(m *Model, input string) bool {
	parts := strings.Split(input, " ")

	cmd := parts[0]

	switch cmd {

	case "/clear":
		m.messages = []string{}
		m.viewport.SetContent("")
		return true

	case "/quit":
		clearTerminal()
		return false

	case "/help":
		m.messages = append(m.messages,
			systemStyle.Render(
				"Commands: /help /clear /nick /color /users /quit",
			),
		)

		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()

		return true

	case "/nick":
		if len(parts) < 2 {
			return true
		}

		newNick := parts[1]

		m.conn.conn.WriteJSON(Message{
			Type:    "nick",
			NewNick: newNick,
		})

		m.nick = newNick

		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()

		return true

	case "/users":
		m.conn.conn.WriteJSON(Message{
			Type: "users",
		})

		return true

	case "/color":
		if len(parts) < 2 {
			return true
		}

		color := parts[1]

		if !isValidHexColor(color) {
			m.messages = append(
				m.messages,
				systemStyle.Render("Invalid hex color"),
			)

			m.viewport.SetContent(strings.Join(m.messages, "\n"))
			m.viewport.GotoBottom()

			return true
		}

		m.conn.conn.WriteJSON(Message{
			Type:  "color",
			Color: color,
		})

		return true
	}

	return true
}

func clearTerminal() {
	print("\033[H\033[2J")
}

func isValidHexColor(color string) bool {
	re := regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	return re.MatchString(color)
}

package main

import (
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
			return m, tea.Quit

		case "enter":
			text := m.input.Value()

			if text != "" {
				m.conn.conn.WriteJSON(Message{
					Type: "message",
					Text: text,
				})

				m.input.SetValue("")
			}
		}

	case IncomingMessage:
		switch msg.Type {

		case "system":
			formatted := systemStyle.Render("[system] " + msg.Text)
			if m.viewport.Width > 0 {
				formatted = wordwrap.String(formatted, m.viewport.Width)
			}
			m.messages = append(m.messages, formatted)

		case "message":
			formatted := nickStyle.Render(msg.Nick) + ": " + msg.Text
			if m.viewport.Width > 0 {
				formatted = wordwrap.String(formatted, m.viewport.Width)
			}
			m.messages = append(m.messages, formatted)
		}

		m.viewport.SetContent(strings.Join(m.messages, "\n"))
		m.viewport.GotoBottom()

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

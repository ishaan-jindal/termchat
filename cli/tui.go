package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type IncomingMessage Message

type Model struct {
	conn *Connection

	messages []string
	input    textinput.Model

	nick string
	room string
}

func NewModel(conn *Connection, nick string, room string) Model {
	ti := textinput.New()

	ti.Placeholder = "Type a message..."
	ti.Focus()

	return Model{
		conn:     conn,
		messages: []string{},
		input:    ti,
		nick:     nick,
		room:     room,
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
			m.messages = append(m.messages, "[system] "+msg.Text)

		case "message":
			m.messages = append(
				m.messages,
				msg.Nick+": "+msg.Text,
			)
		}

		return m, waitForMessage(m.conn)
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

func (m Model) View() string {
	var output strings.Builder
	output.WriteString("Room: " + m.room + "\n\n")

	for _, msg := range m.messages {
		output.WriteString(msg + "\n")
	}

	output.WriteString("\n> " + m.input.View())

	return output.String()
}

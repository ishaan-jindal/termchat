package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveThemeNamedPalettes(t *testing.T) {
	for _, name := range []string{"dark", "light", "dracula", "nord", "gruvbox"} {
		theme, err := resolveTheme(name)
		if err != nil {
			t.Fatalf("resolveTheme(%q) = %v", name, err)
		}

		if theme.Name != name {
			t.Errorf("theme.Name = %q, want %q", theme.Name, name)
		}
	}
}

func TestResolveThemeUnknownListsValid(t *testing.T) {
	_, err := resolveTheme("nope")
	if err == nil {
		t.Fatal("expected error for unknown theme")
	}

	for _, name := range themeNames {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not list %q", err.Error(), name)
		}
	}
}

func TestResolveThemeSystem(t *testing.T) {
	// Without a TTY the background detection falls back to a default;
	// either variant is a valid resolution.
	theme, err := resolveTheme("system")
	if err != nil {
		t.Fatal(err)
	}

	if theme.Name != "dark" && theme.Name != "light" {
		t.Fatalf("system resolved to %q", theme.Name)
	}
}

func TestParseThemeFlag(t *testing.T) {
	opts, err := parseArgs([]string{"--theme", "gruvbox"})
	if err != nil {
		t.Fatal(err)
	}

	if opts.Theme != "gruvbox" {
		t.Fatalf("theme = %q, want gruvbox", opts.Theme)
	}

	opts, err = parseArgs([]string{"--theme=dracula"})
	if err != nil {
		t.Fatal(err)
	}

	if opts.Theme != "dracula" {
		t.Fatalf("inline theme = %q, want dracula", opts.Theme)
	}
}

func TestViewPaintsFullCanvas(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})

	lines := strings.Split(m.View(), "\n")

	if len(lines) != 24 {
		t.Fatalf("view height = %d, want 24", len(lines))
	}

	for i, line := range lines {
		if !strings.Contains(line, ";48;5;235m") {
			t.Errorf("view line %d not painted with theme background", i)
		}
	}
}

// The reset ending a colored nick span must not drop the theme background
// for the remainder of the line.
func TestMessageTailCarriesThemeBackground(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, _ = update(t, m, IncomingMessage(Message{
		Type:  "message",
		ID:    1,
		Nick:  "bob",
		Color: "#aa0000",
		Text:  "hello world",
	}))

	line := m.messages[0].rendered
	idx := strings.Index(line, "\x1b[0m")

	if idx < 0 || !strings.Contains(line[idx:], ";48;5;235m") {
		t.Errorf("message tail lost the theme background: %q", line)
	}
}

func TestSelectedCompletionRowFullyPainted(t *testing.T) {
	forceColor(t)

	m := testModel()
	m.input.SetValue("/")
	refreshCompletion(&m)

	popup := renderCompletion(m)

	if !m.showPopup {
		t.Fatal("popup did not open")
	}

	if got := strings.Count(popup, ";48;5;238m"); got < 2 {
		t.Errorf("selected row painted %d segments, want both columns: %q", got, popup)
	}
}

func TestInputUsesThemeStyles(t *testing.T) {
	forceColor(t)

	m := testModel()

	if v := m.input.View(); !strings.Contains(v, ";48;5;235m") {
		t.Errorf("input box not themed: %q", v)
	}
}

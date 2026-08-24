package main

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

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

// A reset followed by plain spaces means those cells fall back to the
// terminal colors; the view must never contain such a gap. A second scan
// tracks the active SGR background so foreground-only spans (which let the
// terminal background bleed through behind text) are caught too.
func TestViewHasNoUnpaintedGaps(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = update(t, m, IncomingMessage(Message{
		Type:  "message",
		ID:    1,
		Nick:  "bob",
		Color: "#aa0000",
		Text:  "hello world",
	}))
	m.input.SetValue("/")
	refreshCompletion(&m)

	view := m.View()

	bleed := regexp.MustCompile("\x1b\\[0m[ ]{2}")
	if bleed.MatchString(view) {
		t.Error("view contains unpainted space after a reset sequence")
	}

	if idx := unpaintedRuneIndex(view); idx >= 0 {
		start := max(idx-40, 0)
		end := min(idx+20, len(view))

		t.Errorf("unpainted text at %d: %q", idx, view[start:end])
	}
	ansiSeq := regexp.MustCompile("\x1b\\[[0-9;]*m")
	lines := strings.Split(ansiSeq.ReplaceAllString(view, ""), "\n")

	for i, line := range lines {
		if width := utf8.RuneCountInString(line); width != 120 {
			t.Fatalf("line %d visible width = %d, want 120", i, width)
		}
	}
}

// unpaintedRuneIndex returns the offset of the first visible non-space rune
// rendered without an active SGR background, or -1.
func unpaintedRuneIndex(s string) int {
	seq := regexp.MustCompile("\x1b\\[([0-9;]*)m")
	bg := false

	i := 0

	for i < len(s) {
		if s[i] == 0x1b {
			loc := seq.FindStringSubmatchIndex(s[i:])
			if loc == nil || loc[0] != 0 {
				i++

				continue
			}

			for _, p := range strings.Split(s[i+loc[2]:i+loc[3]], ";") {
				switch p {
				case "", "0", "49":
					bg = false
				case "40", "41", "42", "43", "44", "45", "46", "47",
					"100", "101", "102", "103", "104", "105", "106", "107", "48":
					bg = true
				}
			}

			i += loc[1]

			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if (r != ' ' && r != '\n' && r != '\r') && !bg {
			return i
		}

		i += size
	}

	return -1
}

func TestStatusBarSpansFullWidth(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	ansiSeq := regexp.MustCompile("\x1b\\[[0-9;]*m")

	var strip string

	found := false

	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, ";48;5;236m") {
			strip = line
			found = true
		}
	}

	if !found {
		t.Fatal("status bar strip not painted")
	}

	if width := utf8.RuneCountInString(ansiSeq.ReplaceAllString(strip, "")); width != 120 {
		t.Errorf("status bar width = %d, want 120", width)
	}
}

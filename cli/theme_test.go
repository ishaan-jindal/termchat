package main

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveThemeNamedPalettes(t *testing.T) {
	for _, nt := range builtinThemes {
		theme, err := resolveTheme(nt.name)
		if err != nil {
			t.Fatalf("resolveTheme(%q) = %v", nt.name, err)
		}

		if theme.Name != nt.name {
			t.Errorf("theme.Name = %q, want %q", theme.Name, nt.name)
		}
	}
}

// Every registered palette must be complete: a missing role compiles fine
// but renders as an unpainted hole in the window.
func TestThemeRegistry(t *testing.T) {
	seen := map[string]bool{}

	for _, nt := range builtinThemes {
		if nt.name == "" {
			t.Fatal("registry contains an empty theme name")
		}

		if seen[nt.name] {
			t.Errorf("duplicate theme name %q", nt.name)
		}

		seen[nt.name] = true

		p := reflect.ValueOf(nt.palette)

		for i := 0; i < p.NumField(); i++ {
			if p.Field(i).String() == "" {
				t.Errorf("%s: role %s is empty", nt.name, p.Type().Field(i).Name)
			}
		}
	}

	if !seen["dark"] || !seen["light"] {
		t.Error("registry must contain dark and light")
	}
}

// Every registered theme must paint the whole canvas: new palettes inherit
// the bleed-proofing automatically.
func TestEveryThemePaintsCanvas(t *testing.T) {
	forceColor(t)

	for _, nt := range builtinThemes {
		m := NewModel(&Connection{
			Send: make(chan Message, 32),
			done: make(chan struct{}),
		}, "alice", "TEST", registeredTheme(nt.name))

		m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
		m, _ = update(t, m, IncomingMessage(Message{
			Type:  "message",
			ID:    1,
			Nick:  "bob",
			Color: "#aa0000",
			Text:  "hello world",
		}))

		view := m.View()

		if idx := unpaintedRuneIndex(view); idx >= 0 {
			start := max(idx-40, 0)
			end := min(idx+20, len(view))

			t.Errorf("[%s] unpainted cell at %d: %q", nt.name, idx, view[start:end])
		}
	}
}

func TestResolveThemeUnknownListsValid(t *testing.T) {
	_, err := resolveTheme("nope")
	if err == nil {
		t.Fatal("expected error for unknown theme")
	}

	for _, name := range themeNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("error %q does not list %q", err.Error(), name)
		}
	}
}

func TestResolveThemeSystem(t *testing.T) {
	// system keeps the terminal's own colors: the base style must not
	// emit any SGR at all.
	theme, err := resolveTheme("system")
	if err != nil {
		t.Fatal(err)
	}

	if theme.Name != "system" {
		t.Fatalf("system resolved to %q", theme.Name)
	}

	if theme.base.Render("x") != "x" {
		t.Errorf("system base is not transparent: %q", theme.base.Render("x"))
	}
}

func TestSwitchToSystemDropsAllStyling(t *testing.T) {
	forceColor(t)

	m := testModel()

	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = update(t, m, IncomingMessage(Message{Type: "message", ID: 1, Nick: "bob", Color: "#aa0000", Text: "hello world"}))

	handled, quit := handleCommand(&m, "/theme system")

	if !handled || quit {
		t.Fatalf("handled = %v, quit = %v", handled, quit)
	}

	if m.theme.Name != "system" {
		t.Fatalf("theme = %q, want system", m.theme.Name)
	}

	view := m.View()

	// Adaptive accents keep their own backgrounds (status bar, headers);
	// the dark canvas and input colors must be gone entirely.
	if strings.Contains(view, ";48;5;235") || strings.Contains(view, "38;5;252;48") {
		t.Errorf("system view still forces the dark canvas: %q", view)
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

// Both cursor phases paint: visible reverses the theme colors and hidden
// renders the character under it through the theme.
func TestCursorUsesThemeStyles(t *testing.T) {
	forceColor(t)

	m := testModel()
	m.input.SetValue("abc")

	m.input.Cursor.Blink = true

	if v := m.input.View(); !strings.Contains(v, ";48;5;235m") {
		t.Errorf("hidden cursor not themed: %q", v)
	}

	m.input.Cursor.Blink = false
	m.input.Cursor.SetChar("x")

	visible := m.input.View()

	if !strings.Contains(visible, ";48;5;235m") {
		t.Errorf("visible cursor lost theme background: %q", visible)
	}

	if !strings.Contains(visible, "\x1b[7;") && !strings.Contains(visible, "\x1b[7m") {
		t.Errorf("visible cursor not reversed: %q", visible)
	}
}

// A reset followed by plain spaces means those cells fall back to the
// terminal colors; the view must never contain such a gap. A second scan
// tracks the active SGR background so foreground-only spans (which let the
// terminal background bleed through behind text) are caught too.
func TestViewHasNoUnpaintedGaps(t *testing.T) {
	forceColor(t)

	states := map[string]func(m *Model){
		"placeholder": func(m *Model) {},
		"typed": func(m *Model) {
			m.input.SetValue("hello world")
			m.input.SetCursor(5)
		},
	}

	for name, prepare := range states {
		for _, blink := range []bool{true, false} {
			m := testModel()

			m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
			m, _ = update(t, m, IncomingMessage(Message{
				Type:  "message",
				ID:    1,
				Nick:  "bob",
				Color: "#aa0000",
				Text:  "hello world",
			}))

			prepare(&m)
			m.input.Cursor.Blink = blink

			view := m.View()

			if idx := unpaintedRuneIndex(view); idx >= 0 {
				start := max(idx-40, 0)
				end := min(idx+20, len(view))

				t.Errorf("%s (blink %v): unpainted text at %d: %q", name, blink, idx, view[start:end])
			}

			bareSpaceRe2 := regexp.MustCompile("\x1b\\[0m[ ]{2}")
			if bareSpaceRe2.MatchString(view) {
				t.Errorf("%s (blink %v): view contains plain spaces after a reset", name, blink)
			}
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

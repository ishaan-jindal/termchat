package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// palette holds the raw color roles every theme must define. bg/fg paint the
// whole window so named themes take over from the terminal colors.
type palette struct {
	bg         string
	fg         string
	dim        string
	mentionBg  string
	mentionFg  string
	border     string
	statusBg   string
	statusFg   string
	selectedBg string
	headerBg   string
	headerFg   string
}

var palettes = map[string]palette{
	"dark": {
		bg:         "235",
		fg:         "252",
		dim:        "8",
		mentionBg:  "255",
		mentionFg:  "0",
		border:     "238",
		statusBg:   "236",
		statusFg:   "250",
		selectedBg: "238",
		headerBg:   "238",
		headerFg:   "15",
	},
	"light": {
		bg:         "255",
		fg:         "234",
		dim:        "242",
		mentionBg:  "235",
		mentionFg:  "15",
		border:     "248",
		statusBg:   "254",
		statusFg:   "235",
		selectedBg: "249",
		headerBg:   "250",
		headerFg:   "0",
	},
	"dracula": {
		bg:         "#282a36",
		fg:         "#f8f8f2",
		dim:        "#6272a4",
		mentionBg:  "#44475a",
		mentionFg:  "#f8f8f2",
		border:     "#bd93f9",
		statusBg:   "#44475a",
		statusFg:   "#f8f8f2",
		selectedBg: "#44475a",
		headerBg:   "#bd93f9",
		headerFg:   "#282a36",
	},
	"nord": {
		bg:         "#2e3440",
		fg:         "#eceff4",
		dim:        "#616e88",
		mentionBg:  "#434c5e",
		mentionFg:  "#eceff4",
		border:     "#4c566a",
		statusBg:   "#3b4252",
		statusFg:   "#eceff4",
		selectedBg: "#434c5e",
		headerBg:   "#88c0d0",
		headerFg:   "#2e3440",
	},
	"gruvbox": {
		bg:         "#282828",
		fg:         "#ebdbb2",
		dim:        "#928374",
		mentionBg:  "#504945",
		mentionFg:  "#fbf1c7",
		border:     "#665c54",
		statusBg:   "#3c3836",
		statusFg:   "#ebdbb2",
		selectedBg: "#504945",
		headerBg:   "#fabd2f",
		headerFg:   "#3c3836",
	},
}

// themeNames is the stable ordering shown by /theme and error messages.
var themeNames = []string{"system", "dark", "light", "dracula", "nord", "gruvbox"}

func validThemes() string {
	return strings.Join(themeNames, ", ")
}

func isThemeName(name string) bool {
	for _, n := range themeNames {
		if n == name {
			return true
		}
	}

	return false
}

// resolveTheme builds the named theme; "system" detects the terminal
// background and falls back to its dark or light variant.
func resolveTheme(name string) (Theme, error) {
	if name == "system" {
		if termenv.HasDarkBackground() {
			return buildTheme("dark"), nil
		}

		return buildTheme("light"), nil
	}

	if !isThemeName(name) {
		return Theme{}, fmt.Errorf("unknown theme %q (valid: %s)", name, validThemes())
	}

	return buildTheme(name), nil
}

func buildTheme(name string) Theme {
	p := palettes[name]

	t := Theme{Name: name}

	t.base = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.fg)).
		Background(lipgloss.Color(p.bg))
	t.system = t.base.Foreground(lipgloss.Color(p.dim))
	t.mention = lipgloss.NewStyle().
		Background(lipgloss.Color(p.mentionBg)).
		Foreground(lipgloss.Color(p.mentionFg)).
		Bold(true)
	t.panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Background(lipgloss.Color(p.bg))
	if p.border != "" {
		// Border runes need their own background or the terminal's
		// bleeds through behind them.
		t.panel = t.panel.
			BorderForeground(lipgloss.Color(p.border)).
			BorderBackground(lipgloss.Color(p.bg))
	}
	t.status = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.statusFg)).
		Background(lipgloss.Color(p.statusBg)).
		Padding(0, 1)
	t.completionSelected = lipgloss.NewStyle().
		Background(lipgloss.Color(p.selectedBg)).
		Bold(true)
	t.usersHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(p.headerFg)).
		Background(lipgloss.Color(p.headerBg)).
		Padding(0, 1)
	t.input = textarea.Style{
		Base:        t.base,
		Text:        t.base,
		CursorLine:  t.base,
		EndOfBuffer: t.base,
		Placeholder: t.system,
		Prompt:      t.base,
	}

	return t
}

// Theme resolves a palette into the styles used across the TUI. base carries
// the theme background so every rendered cell can be painted explicitly;
// canvas sizes it to the full terminal in View.
type Theme struct {
	Name               string
	base               lipgloss.Style
	system             lipgloss.Style
	mention            lipgloss.Style
	panel              lipgloss.Style
	status             lipgloss.Style
	completionSelected lipgloss.Style
	usersHeader        lipgloss.Style
	input              textarea.Style
}

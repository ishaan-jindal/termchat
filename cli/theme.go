package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
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

// namedPalette pairs a display name with its color roles; the slice order is
// what /theme and error messages show.
//
// Adding a theme: append an entry here with every role filled in. Values
// accept ANSI 256 indices ("235") or hex ("#bd93f9"). Roles:
//
//	bg/fg - window canvas          dim - system lines, reactions, quotes
//	mention* - @mention block      border - panel frames
//	status* - footer bar           selectedBg - autocomplete selection
//	header* - sidebar title
type namedPalette struct {
	name    string
	palette palette
}

var builtinThemes = []namedPalette{
	{"dark", palette{
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
	}},
	{"light", palette{
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
	}},
	{"dracula", palette{
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
	}},
	{"nord", palette{
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
	}},
	{"gruvbox", palette{
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
	}},
}

// themeNames lists "system" followed by every registered theme.
func themeNames() []string {
	names := make([]string, 0, len(builtinThemes)+1)
	names = append(names, "system")

	for _, t := range builtinThemes {
		names = append(names, t.name)
	}

	return names
}

func validThemes() string {
	return strings.Join(themeNames(), ", ")
}

func isThemeName(name string) bool {
	for _, n := range themeNames() {
		if n == name {
			return true
		}
	}

	return false
}

// lookupPalette finds a registered palette by name.
func lookupPalette(name string) (palette, bool) {
	for _, t := range builtinThemes {
		if t.name == name {
			return t.palette, true
		}
	}

	return palette{}, false
}

// registeredTheme builds a theme by registry name; only known-good constant
// names reach this.
func registeredTheme(name string) Theme {
	p, ok := lookupPalette(name)
	if !ok {
		panic("unregistered theme " + name)
	}

	return buildTheme(name, p)
}

// themeSwatch renders three chips previewing a theme's canvas, border and
// status colors.
func themeSwatch(p palette) string {
	chip := func(c string) string {
		return lipgloss.NewStyle().Background(lipgloss.Color(c)).Render("  ")
	}

	return chip(p.bg) + " " + chip(p.border) + " " + chip(p.statusBg)
}

// resolveTheme builds the named theme; "system" keeps the terminal's own
// colors and only adapts the accents to light or dark backgrounds.
func resolveTheme(name string) (Theme, error) {
	if name == "system" {
		return buildSystemTheme(), nil
	}

	if !isThemeName(name) {
		return Theme{}, fmt.Errorf("unknown theme %q (valid: %s)", name, validThemes())
	}

	return registeredTheme(name), nil
}

// buildSystemTheme leaves every canvas role unstyled so nothing overrides
// the terminal; accents use adaptive colors for light and dark backgrounds.
func buildSystemTheme() Theme {
	t := Theme{Name: "system"}

	dim := lipgloss.AdaptiveColor{Light: "240", Dark: "8"}

	t.base = lipgloss.NewStyle()
	t.system = t.base.Foreground(dim)
	t.mention = lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "235", Dark: "255"}).
		Foreground(lipgloss.AdaptiveColor{Light: "15", Dark: "0"}).
		Bold(true)
	t.panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1)
	t.status = t.base.
		Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "250"}).
		Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"}).
		Padding(0, 1)
	t.completionSelected = lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "249", Dark: "238"}).
		Bold(true)
	t.usersHeader = t.base.
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "15"}).
		Background(lipgloss.AdaptiveColor{Light: "250", Dark: "238"}).
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

func buildTheme(name string, p palette) Theme {
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

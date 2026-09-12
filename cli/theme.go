package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

// palette holds the raw color roles every theme must define. bg/fg paint the
// whole window so named themes take over from the terminal colors. accent
// roles are hex so the message flash can lerp them.
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
	accent     string
	accentFg   string
	borderDim  string
	hint       string
}

// namedPalette pairs a display name with its color roles in /theme order.
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
		border:     "#444444",
		statusBg:   "236",
		statusFg:   "250",
		selectedBg: "238",
		headerBg:   "238",
		headerFg:   "15",
		accent:     "#5fafff",
		accentFg:   "#121212",
		borderDim:  "#3a3a3a",
		hint:       "#8a8a8a",
	}},
	{"light", palette{
		bg:         "255",
		fg:         "234",
		dim:        "242",
		mentionBg:  "235",
		mentionFg:  "15",
		border:     "#a8a8a8",
		statusBg:   "254",
		statusFg:   "235",
		selectedBg: "249",
		headerBg:   "250",
		headerFg:   "0",
		accent:     "#005fd7",
		accentFg:   "#ffffff",
		borderDim:  "#d0d0d0",
		hint:       "#8a8a8a",
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
		accent:     "#ff79c6",
		accentFg:   "#282a36",
		borderDim:  "#44475a",
		hint:       "#6272a4",
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
		accent:     "#88c0d0",
		accentFg:   "#2e3440",
		borderDim:  "#3b4252",
		hint:       "#616e88",
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
		accent:     "#fabd2f",
		accentFg:   "#3c3836",
		borderDim:  "#504945",
		hint:       "#928374",
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

// registeredTheme builds a theme by registry name, falling back to dark.
func registeredTheme(name string) Theme {
	p, ok := lookupPalette(name)
	if !ok {
		p, _ = lookupPalette("dark")
	}

	return buildTheme(name, p)
}

// themeSwatch renders four contiguous chips previewing a theme's canvas,
// border, status and accent colors.
func themeSwatch(p palette) string {
	chip := func(c string) string {
		return lipgloss.NewStyle().Background(lipgloss.Color(c)).Render("   ")
	}

	return chip(p.bg) + chip(p.border) + chip(p.statusBg) + chip(p.accent)
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

// buildSystemTheme keeps the terminal's palette spirit but paints every
// role explicitly, so panels never show raw terminal colors. All accents
// adapt to light and dark backgrounds.
func buildSystemTheme() Theme {
	t := Theme{Name: "system"}

	bg := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#121212"}
	fg := lipgloss.AdaptiveColor{Light: "#1c1c1c", Dark: "#e4e4e4"}
	dim := lipgloss.AdaptiveColor{Light: "240", Dark: "8"}
	border := lipgloss.AdaptiveColor{Light: "#d0d0d0", Dark: "#3a3a3a"}
	accent := lipgloss.AdaptiveColor{Light: "#005fd7", Dark: "#5fafff"}
	accentFg := lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#121212"}

	t.base = lipgloss.NewStyle().Foreground(fg).Background(bg)
	t.system = t.base.Foreground(dim)
	t.mention = lipgloss.NewStyle().
		Background(lipgloss.AdaptiveColor{Light: "235", Dark: "255"}).
		Foreground(lipgloss.AdaptiveColor{Light: "15", Dark: "0"}).
		Bold(true)
	t.panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Background(bg).
		BorderForeground(border).
		BorderBackground(bg)
	t.status = t.base.
		Foreground(lipgloss.AdaptiveColor{Light: "235", Dark: "250"}).
		Background(lipgloss.AdaptiveColor{Light: "254", Dark: "236"}).
		Padding(0, 1)
	t.completionSelected = lipgloss.NewStyle().
		Foreground(fg).
		Background(lipgloss.AdaptiveColor{Light: "249", Dark: "238"}).
		Bold(true)
	t.usersHeader = t.base.
		Bold(true).
		Foreground(lipgloss.AdaptiveColor{Light: "0", Dark: "15"}).
		Background(lipgloss.AdaptiveColor{Light: "250", Dark: "238"}).
		Padding(0, 1)
	t.accent = lipgloss.NewStyle().
		Foreground(accent).
		Bold(true)
	t.accentBg = lipgloss.NewStyle().
		Foreground(accentFg).
		Background(accent).
		Bold(true)
	t.hint = t.base.Foreground(dim)
	t.statusHint = t.status.Padding(0).Foreground(dim)
	t.statusKey = t.status.Padding(0).Foreground(accent).Bold(true)
	t.cursor = lipgloss.NewStyle().
		Foreground(accent).
		Background(bg).
		Bold(true)
	t.input = textarea.Style{
		Base:        t.base,
		Text:        t.base,
		CursorLine:  t.base,
		EndOfBuffer: t.base,
		Placeholder: t.system,
		Prompt:      t.base,
	}

	// The animation lerps plain hex; resolve the adaptive pair to the dark
	// variant, which is also what the forced-color tests observe.
	t.accentHex = "#5fafff"
	t.borderDimHex = "#262626"
	t.bgHex = "#121212"

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
		Foreground(lipgloss.Color(p.fg)).
		Background(lipgloss.Color(p.selectedBg)).
		Bold(true)
	t.usersHeader = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(p.headerFg)).
		Background(lipgloss.Color(p.headerBg)).
		Padding(0, 1)
	t.accent = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.accent)).
		Bold(true)
	t.accentBg = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.accentFg)).
		Background(lipgloss.Color(p.accent)).
		Bold(true)
	t.hint = t.base.Foreground(lipgloss.Color(p.hint))
	t.statusHint = t.status.Padding(0).Foreground(lipgloss.Color(p.hint))
	t.statusKey = t.status.Padding(0).Foreground(lipgloss.Color(p.accent)).Bold(true)
	t.cursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color(p.accent)).
		Background(lipgloss.Color(p.bg)).
		Bold(true)

	t.accentHex = p.accent
	t.borderDimHex = p.borderDim
	t.bgHex = ansi256ToHex(p.bg)

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

// Theme is the resolved style set used across the TUI.
type Theme struct {
	Name               string
	base               lipgloss.Style
	system             lipgloss.Style
	mention            lipgloss.Style
	panel              lipgloss.Style
	status             lipgloss.Style
	completionSelected lipgloss.Style
	usersHeader        lipgloss.Style
	accent             lipgloss.Style
	accentBg           lipgloss.Style
	hint               lipgloss.Style
	statusHint         lipgloss.Style
	statusKey          lipgloss.Style
	cursor             lipgloss.Style
	input              textarea.Style

	accentHex    string
	borderDimHex string
	bgHex        string
}

// ansi256ToHex resolves a palette color to hex: hex passes through,
// ANSI 256 indices map via the standard grayscale/color cube. Unknown
// values yield black.
func ansi256ToHex(c string) string {
	if strings.HasPrefix(c, "#") {
		return c
	}

	n, err := strconv.Atoi(c)
	if err != nil || n < 0 || n > 255 {
		return "#000000"
	}

	if n < 16 {
		std := [16][3]int{
			{0, 0, 0}, {205, 0, 0}, {0, 205, 0}, {205, 205, 0},
			{0, 0, 238}, {205, 0, 205}, {0, 205, 205}, {229, 229, 229},
			{127, 127, 127}, {255, 0, 0}, {0, 255, 0}, {255, 255, 0},
			{92, 92, 255}, {255, 0, 255}, {0, 255, 255}, {255, 255, 255},
		}
		rgb := std[n]

		return fmt.Sprintf("#%02x%02x%02x", rgb[0], rgb[1], rgb[2])
	}

	if n < 232 {
		n -= 16
		r := n / 36
		g := (n % 36) / 6
		b := n % 6

		levels := [6]int{0, 95, 135, 175, 215, 255}

		return fmt.Sprintf("#%02x%02x%02x", levels[r], levels[g], levels[b])
	}

	v := 8 + (n-232)*10

	return fmt.Sprintf("#%02x%02x%02x", v, v, v)
}

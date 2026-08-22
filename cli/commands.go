package main

import (
	"fmt"
	"strconv"
	"strings"

	"termchat/shared"
)

type command struct {
	name        string
	usage       string
	description string
	handler     func(m *Model, args []string) (handled bool, quit bool)
}

// Handlers are wired in init because their bodies refer back to commands,
// which would be an initialization cycle with a literal.
var commands []command

func init() {
	commands = []command{
		{
			name:        "/help",
			usage:       "/help",
			description: "list commands",
			handler:     cmdHelp,
		},
		{
			name:        "/clear",
			usage:       "/clear",
			description: "clear the message view",
			handler:     cmdClear,
		},
		{
			name:        "/nick",
			usage:       "/nick <nick>",
			description: "change your nickname",
			handler:     cmdNick,
		},
		{
			name:        "/color",
			usage:       "/color <hex>",
			description: "set your name color",
			handler:     cmdColor,
		},
		{
			name:        "/theme",
			usage:       "/theme [name]",
			description: "list or switch the color theme",
			handler:     cmdTheme,
		},
		{
			name:        "/password",
			usage:       "/password [pass]",
			description: "set or remove room password",
			handler:     cmdPassword,
		},
		{
			name:        "/users",
			usage:       "/users",
			description: "print the room roster",
			handler:     cmdUsers,
		},
		{
			name:        "/reply",
			usage:       "/reply <id> <text>",
			description: "reply to a message by id",
			handler:     cmdReply,
		},
		{
			name:        "/react",
			usage:       "/react <id> <name>",
			description: "react to a message by id",
			handler:     cmdReact,
		},
		{
			name:        "/quit",
			usage:       "/quit",
			description: "leave termchat",
			handler:     cmdQuit,
		},
	}
}

func handleCommand(m *Model, input string) (handled bool, quit bool) {
	parts := strings.Split(input, " ")

	cmd, ok := lookupCommand(parts[0])
	if !ok {
		return false, false
	}

	return cmd.handler(m, parts[1:])
}

func lookupCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}

	return command{}, false
}

func filterCommands(prefix string) []command {
	prefix = strings.ToLower(prefix)

	var matches []command

	for _, c := range commands {
		if strings.HasPrefix(c.name, prefix) {
			matches = append(matches, c)
		}
	}

	return matches
}

func maxUsageLen() int {
	width := 0

	for _, c := range commands {
		width = max(width, len(c.usage))
	}

	return width
}

type suggestion struct {
	primary string // left popup column: "/nick <nick>", ":fire:", "+1"
	detail  string // right popup column: description or glyph
	insert  string // what accepting the row puts into the input
}

// Shortcodes align with shared.ReactionNames so /react names double as emoji.
var emojis = []struct {
	name  string
	glyph string
}{
	{"+1", "\U0001f44d"},
	{"-1", "\U0001f44e"},
	{"laugh", "\U0001f606"},
	{"heart", "\u2764\ufe0f"},
	{"wow", "\U0001f62e"},
	{"eyes", "\U0001f440"},
	{"fire", "\U0001f525"},
	{"clap", "\U0001f44f"},
	{"smile", "\U0001f604"},
	{"joy", "\U0001f602"},
	{"grin", "\U0001f601"},
	{"wink", "\U0001f609"},
	{"tada", "\U0001f389"},
	{"rocket", "\U0001f680"},
	{"thinking", "\U0001f914"},
	{"pray", "\U0001f64f"},
	{"sob", "\U0001f62d"},
	{"100", "\U0001f4af"},
	{"wave", "\U0001f44b"},
	{"sunglasses", "\U0001f60e"},
}

// matchSuggestions decides which completion mode applies to the input and
// returns its rows plus the length of the token an accept would replace.
func matchSuggestions(value string) ([]suggestion, int) {
	switch {

	case strings.HasPrefix(value, "/") && !strings.ContainsAny(value, " \t\n"):
		return commandSuggestions(value), len(value)

	case strings.HasPrefix(value, "/react "):
		parts := strings.Split(value, " ")

		if len(parts) == 3 {
			if _, err := strconv.ParseInt(parts[1], 10, 64); err == nil {
				return reactionSuggestions(parts[2]), len(parts[2])
			}
		}

		return nil, 0

	default:
		token := value[strings.LastIndexAny(value, " \t\n")+1:]

		if strings.HasPrefix(token, ":") && !strings.Contains(token[1:], ":") {
			query := strings.ToLower(strings.TrimPrefix(token, ":"))

			return emojiSuggestions(query), len(token)
		}

		return nil, 0
	}
}

func commandSuggestions(prefix string) []suggestion {
	var out []suggestion

	for _, c := range filterCommands(prefix) {
		out = append(out, suggestion{
			primary: c.usage,
			detail:  c.description,
			insert:  c.name + " ",
		})
	}

	return out
}

// emojiGlyph returns the glyph for a shortcode name, or empty if unknown.
func emojiGlyph(name string) string {
	for _, e := range emojis {
		if e.name == name {
			return e.glyph
		}
	}

	return ""
}

func reactionSuggestions(query string) []suggestion {
	query = strings.ToLower(query)

	var out []suggestion

	for _, name := range shared.ReactionNames {
		if !strings.HasPrefix(strings.ToLower(name), query) {
			continue
		}

		out = append(out, suggestion{
			primary: ":" + name + ":",
			detail:  emojiGlyph(name),
			insert:  name,
		})
	}

	return out
}

func emojiSuggestions(query string) []suggestion {
	var out []suggestion

	for _, e := range emojis {
		if strings.HasPrefix(e.name, query) {
			out = append(out, suggestion{
				primary: ":" + e.name + ":",
				detail:  e.glyph,
				insert:  e.glyph + " ",
			})
		}
	}

	return out
}

// trySend delivers a frame without ever blocking the TUI.
func trySend(m *Model, msg Message) {
	select {
	case m.conn.Send <- msg:
	default:
	}
}

func cmdHelp(m *Model, _ []string) (bool, bool) {
	var b strings.Builder

	b.WriteString(m.theme.system.Render("Commands:"))

	for _, c := range commands {
		b.WriteString("\n")
		b.WriteString(m.theme.system.Render(fmt.Sprintf("%-*s", maxUsageLen(), c.usage)))
		b.WriteString("  ")
		b.WriteString(c.description)
	}

	appendUI(m, b.String())

	return true, false
}

func cmdClear(m *Model, _ []string) (bool, bool) {
	m.messages = []chatLine{}
	m.msgIndex = map[int64]int{}
	m.viewport.SetContent("")

	return true, false
}

func cmdQuit(_ *Model, _ []string) (bool, bool) {
	return true, true
}

func cmdUsers(m *Model, _ []string) (bool, bool) {
	select {
	case m.conn.Send <- Message{
		Type: "users",
	}:
		m.usersRequested = true
	default:
	}

	return true, false
}

func cmdNick(m *Model, args []string) (bool, bool) {
	if len(args) < 1 {
		return true, false
	}

	newNick := args[0]

	trySend(m, Message{
		Type:    "nick",
		NewNick: newNick,
	})

	m.nick = newNick

	return true, false
}

func cmdColor(m *Model, args []string) (bool, bool) {
	if len(args) < 1 {
		return true, false
	}

	color := args[0]

	if !shared.IsValidHexColor(color) {
		appendUI(m, "Invalid hex color")
		return true, false
	}

	trySend(m, Message{
		Type:  "color",
		Color: color,
	})

	cfg := loadConfig()
	cfg.Color = color
	saveConfig(cfg)

	return true, false
}

// cmdTheme lists themes, or switches to the named one and persists it.
func cmdTheme(m *Model, args []string) (bool, bool) {
	if len(args) < 1 {
		appendUI(m, themeList(m.theme.Name))
		return true, false
	}

	theme, err := resolveTheme(args[0])
	if err != nil {
		appendUI(m, err.Error())
		return true, false
	}

	m.theme = theme
	rerenderAll(m)

	cfg := loadConfig()
	cfg.Theme = args[0]
	saveConfig(cfg)

	appendUI(m, "Theme set to "+theme.Name)

	return true, false
}

// themeList shows every theme name, marking the active one with "*".
func themeList(active string) string {
	parts := make([]string, 0, len(themeNames))

	for _, n := range themeNames {
		if n == active {
			parts = append(parts, "* "+n)
			continue
		}

		parts = append(parts, n)
	}

	return "Themes: " + strings.Join(parts, ", ")
}

func cmdPassword(m *Model, args []string) (bool, bool) {
	if len(args) < 1 {
		// No argument means remove the password
		trySend(m, Message{
			Type:     "set_password",
			Password: "",
		})
		return true, false
	}

	newPass := strings.Join(args, " ")

	trySend(m, Message{
		Type:     "set_password",
		Password: newPass,
	})

	return true, false
}

func cmdReply(m *Model, args []string) (bool, bool) {
	if len(args) < 2 {
		return true, false
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return true, false
	}

	text := strings.TrimSpace(strings.Join(args[1:], " "))
	if text == "" {
		return true, false
	}

	trySend(m, Message{
		Type:      "message",
		Text:      text,
		ReplyToID: id,
	})

	return true, false
}

func cmdReact(m *Model, args []string) (bool, bool) {
	if len(args) < 2 {
		return true, false
	}

	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return true, false
	}

	name := args[1]

	if !shared.IsValidReaction(name) {
		appendUI(m, "Invalid reaction: "+name)
		return true, false
	}

	trySend(m, Message{
		Type: "reaction",
		ID:   id,
		Text: name,
	})

	return true, false
}

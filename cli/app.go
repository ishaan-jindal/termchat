package main

import (
	"fmt"
	"strings"
	"time"

	"termchat/shared"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type appScreen int

const (
	screenHome appScreen = iota
	screenChat
)

const (
	focusRoom = iota
	focusNick
	focusPass
	focusList
)

// hubRow is one selectable discover row: an online room or a LAN beacon.
type hubRow struct {
	online bool
	room   string
	host   string
	users  int
	locked bool
	addr   string
	port   int
}

type onlineRoomsMsg struct {
	rooms []shared.RoomInfo
	err   error
}

type lanRoomsMsg struct {
	beacons []lanBeacon
}

type hostedMsg struct {
	errs <-chan error
}

type hostErrorMsg struct {
	err error
}

// scanOnlineCmd fetches the cloud room list without blocking the TUI.
func scanOnlineCmd(base string) tea.Cmd {
	return func() tea.Msg {
		rooms, err := fetchOnlineRooms(base)

		return onlineRoomsMsg{rooms: rooms, err: err}
	}
}

// scanLANCmd listens for host beacons; it blocks ~3s like discover --local.
func scanLANCmd() tea.Cmd {
	return func() tea.Msg {
		return lanRoomsMsg{beacons: listenForBeacons(3 * time.Second)}
	}
}

// hostServerCmd starts the embedded LAN server off the UI thread.
func hostServerCmd(port int, password string) tea.Cmd {
	return func() tea.Msg {
		errs, err := startLocalServer(port, password)
		if err != nil {
			return hostErrorMsg{err: err}
		}

		return hostedMsg{errs: errs}
	}
}

// hubOptions carries the launch configuration main parsed from argv.
type hubOptions struct {
	room       string
	fresh      bool
	password   string
	serverURL  string
	base       string
	hostMode   bool
	port       int
	showOnline bool
	showLocal  bool
	cfg        Config
	theme      Theme
}

type appModel struct {
	screen appScreen

	serverURL string
	base      string
	fresh     bool
	hostMode  bool
	port      int

	showOnline bool
	showLocal  bool

	cfg   Config
	theme Theme

	room  textinput.Model
	nick  textinput.Model
	pass  textinput.Model
	focus int

	online     []shared.RoomInfo
	onlineErr  string
	onlineDone bool
	lan        []lanBeacon
	lanDone    bool
	rows       []hubRow
	sel        int

	spin     spinner.Model
	busy     bool
	busyText string
	errLine  string

	conn *Connection
	chat Model

	width  int
	height int
}

func newAppModel(o hubOptions) appModel {
	room := textinput.New()
	room.Prompt = ""
	room.Placeholder = "ABCD"
	room.CharLimit = 4
	room.SetValue(o.room)

	nick := o.cfg.Nick
	if !shared.IsValidNickname(nick) {
		nick = "anonymous"
	}

	nickInput := textinput.New()
	nickInput.Prompt = ""
	nickInput.Placeholder = "anonymous"
	nickInput.CharLimit = shared.MaxNicknameLength
	nickInput.SetValue(nick)

	pass := textinput.New()
	pass.Prompt = ""
	pass.Placeholder = "none"
	pass.EchoMode = textinput.EchoPassword
	pass.EchoCharacter = '*'
	pass.SetValue(o.password)

	sp := spinner.New()
	sp.Style = o.theme.system

	m := appModel{
		serverURL:  o.serverURL,
		base:       o.base,
		fresh:      o.fresh,
		hostMode:   o.hostMode,
		port:       o.port,
		showOnline: o.showOnline,
		showLocal:  o.showLocal,
		cfg:        o.cfg,
		theme:      o.theme,
		room:       room,
		nick:       nickInput,
		pass:       pass,
		spin:       sp,
	}

	m.applyHubInputStyles()

	// The discover flow lands on the room list; a known room lands on the
	// nickname field; a fresh room keeps the code field focused for edits.
	switch {
	case o.room == "" && !o.fresh:
		m.focus = focusList
	case o.fresh:
		m.focus = focusRoom
	default:
		m.focus = focusNick
	}

	m.focusInput()

	if o.hostMode {
		m.busy = true
		m.busyText = fmt.Sprintf("Starting local server on :%d...", o.port)
		m.blurHomeInputs()
	}

	return m
}

// applyHubInputStyles wires the active theme into the hub text inputs.
// Values render bold; the focused value is underlined (see styleHubFields).
func (m *appModel) applyHubInputStyles() {
	for _, ti := range []*textinput.Model{&m.room, &m.nick, &m.pass} {
		ti.PromptStyle = m.theme.base
		ti.TextStyle = m.theme.base.Bold(true)
		ti.PlaceholderStyle = m.theme.system
		ti.Cursor.Style = m.theme.base
		ti.Cursor.TextStyle = m.theme.base
	}

	m.styleHubFields()
}

// styleHubFields underlines the focused field's value; the rest stay bold.
func (m *appModel) styleHubFields() {
	m.room.TextStyle = m.theme.base.Bold(true)
	m.nick.TextStyle = m.theme.base.Bold(true)
	m.pass.TextStyle = m.theme.base.Bold(true)

	switch m.focus {
	case focusRoom:
		m.room.TextStyle = m.room.TextStyle.Underline(true)
	case focusNick:
		m.nick.TextStyle = m.nick.TextStyle.Underline(true)
	case focusPass:
		m.pass.TextStyle = m.pass.TextStyle.Underline(true)
	}
}

// focusInput seats focus on the active field; list focus blurs everything.
func (m *appModel) focusInput() tea.Cmd {
	m.blurHomeInputs()
	m.styleHubFields()

	switch m.focus {
	case focusRoom:
		return m.room.Focus()
	case focusNick:
		return m.nick.Focus()
	case focusPass:
		return m.pass.Focus()
	}

	return nil
}

func (m *appModel) blurHomeInputs() {
	m.room.Blur()
	m.nick.Blur()
	m.pass.Blur()
}

func (m *appModel) liveConn() *Connection {
	return m.conn
}

func (m appModel) Init() tea.Cmd {
	var cmds []tea.Cmd

	if m.showOnline {
		cmds = append(cmds, scanOnlineCmd(m.base))
	}

	if m.showLocal {
		cmds = append(cmds, scanLANCmd())
	}

	cmds = append(cmds, spinner.Tick, textinput.Blink)

	if m.hostMode {
		cmds = append(cmds, hostServerCmd(m.port, m.pass.Value()))
	}

	return tea.Batch(cmds...)
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.screen == screenChat {
		nc, cmd := m.chat.Update(msg)
		m.chat = nc.(Model)
		m.conn = m.chat.conn

		return m, cmd
	}

	return m.updateHome(msg)
}

func (m appModel) updateHome(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		width := max(msg.Width-18, 20)
		m.room.Width = min(width, 12)
		m.nick.Width = min(width, 32)
		m.pass.Width = min(width, 32)

		return m, nil

	case tea.KeyMsg:
		return m.updateHomeKey(msg)

	case spinner.TickMsg:
		if m.busy || !m.onlineDone || !m.lanDone {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)

			return m, cmd
		}

		return m, nil

	case onlineRoomsMsg:
		m.online = msg.rooms
		m.onlineErr = ""

		if msg.err != nil {
			m.onlineErr = msg.err.Error()
		}

		m.onlineDone = true
		m.rebuildRows()

		return m, nil

	case lanRoomsMsg:
		m.lan = msg.beacons
		m.lanDone = true
		m.rebuildRows()

		return m, nil

	case hostedMsg:
		return m.onHosted()

	case hostErrorMsg:
		m.busy = false
		m.errLine = "host failed: " + msg.err.Error()

		return m, m.focusInput()

	case joinedMsg:
		return m.enterChat(msg.conn)

	case joinPasswordMsg:
		m.busy = false
		m.errLine = "Room is locked - enter the password"
		m.focus = focusPass

		return m, m.focusInput()

	case joinErrorMsg:
		m.busy = false
		m.errLine = msg.err.Error()

		return m, m.focusInput()
	}

	if m.busy || m.focus == focusList {
		return m, nil
	}

	var cmd tea.Cmd

	switch m.focus {
	case focusRoom:
		m.room, cmd = m.room.Update(msg)
	case focusNick:
		m.nick, cmd = m.nick.Update(msg)
	case focusPass:
		m.pass, cmd = m.pass.Update(msg)
	}

	return m, cmd
}

// updateHomeKey handles hub keys. Typing goes to the focused field; list
// focus drives the discover rows instead.
func (m appModel) updateHomeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		return m, tea.Quit

	case "ctrl+r":
		if m.busy {
			return m, nil
		}

		return m, m.rescan()

	case "tab", "shift+tab":
		if m.busy {
			return m, nil
		}

		if msg.String() == "tab" {
			m.focus = (m.focus + 1) % 4
		} else {
			m.focus = (m.focus + 3) % 4
		}

		return m, m.focusInput()

	case "up", "down":
		if m.busy {
			return m, nil
		}

		if m.focus == focusList {
			m.moveSel(msg.String())

			return m, nil
		}

		if msg.String() == "up" {
			m.focus = (m.focus + 3) % 4
		} else {
			m.focus = (m.focus + 1) % 4
		}

		return m, m.focusInput()

	case "enter":
		if m.busy {
			return m, nil
		}

		if m.focus == focusList && len(m.rows) > 0 {
			return m, m.joinRow(m.rows[m.sel])
		}

		return m, m.startJoin(m.serverURL)

	case "r", "h", "ctrl+h":
		if m.busy || m.focus != focusList {
			break
		}

		if msg.String() == "r" {
			return m, m.rescan()
		}

		if !m.hostMode {
			return m, m.startHosting()
		}
	}

	if m.busy || m.focus == focusList {
		return m, nil
	}

	var cmd tea.Cmd

	switch m.focus {
	case focusRoom:
		m.room, cmd = m.room.Update(msg)
	case focusNick:
		m.nick, cmd = m.nick.Update(msg)
	case focusPass:
		m.pass, cmd = m.pass.Update(msg)
	}

	return m, cmd
}

// moveSel steps the discover cursor, wrapping around the combined rows.
func (m *appModel) moveSel(dir string) {
	if len(m.rows) == 0 {
		return
	}

	if dir == "up" {
		m.sel = (m.sel + len(m.rows) - 1) % len(m.rows)
	} else {
		m.sel = (m.sel + 1) % len(m.rows)
	}
}

// rebuildRows merges the online and LAN results into one cursor list.
func (m *appModel) rebuildRows() {
	var rows []hubRow

	if m.showOnline {
		for _, r := range m.online {
			rows = append(rows, hubRow{
				online: true,
				room:   r.ID,
				host:   r.HostNick,
				users:  r.UserCount,
				locked: r.HasPassword,
			})
		}
	}

	if m.showLocal {
		for _, b := range m.lan {
			rows = append(rows, hubRow{
				room: b.Room,
				host: b.Host,
				addr: b.IP,
				port: b.Port,
			})
		}
	}

	m.rows = rows

	if len(rows) == 0 {
		m.sel = 0
	} else {
		m.sel = min(m.sel, len(rows)-1)
	}
}

// rescan clears the lists and refires both discovery scans.
func (m *appModel) rescan() tea.Cmd {
	m.online = nil
	m.onlineErr = ""
	m.onlineDone = false
	m.lan = nil
	m.lanDone = false
	m.rows = nil
	m.sel = 0

	var cmds []tea.Cmd

	if m.showOnline {
		cmds = append(cmds, scanOnlineCmd(m.base))
	}

	if m.showLocal {
		cmds = append(cmds, scanLANCmd())
	}

	cmds = append(cmds, spinner.Tick)

	return tea.Batch(cmds...)
}

// prepIdentity validates the room and nickname fields, mirroring the old
// startup prompt defaults (blank nick falls back to anonymous).
func (m *appModel) prepIdentity() (string, string, bool) {
	room := shared.NormalizeRoomCode(strings.TrimSpace(m.room.Value()))
	if room == "" {
		room = shared.GenerateRoomCode()
		m.fresh = true
	}

	if !shared.IsValidRoomCode(room) {
		m.errLine = fmt.Sprintf("invalid room code %q", strings.TrimSpace(m.room.Value()))
		m.focus = focusRoom

		return "", "", false
	}

	m.room.SetValue(room)

	nick := strings.TrimSpace(m.nick.Value())
	if nick == "" {
		nick = "anonymous"
		m.nick.SetValue(nick)
	}

	if !shared.IsValidNickname(nick) {
		m.errLine = "Invalid nickname: " + nicknameError(nick)
		m.focus = focusNick

		return "", "", false
	}

	m.cfg.Nick = nick
	saveConfig(m.cfg)

	return room, nick, true
}

// startJoin validates the form and dials the room; rejections surface as
// hub status lines instead of pre-TUI prompts.
func (m *appModel) startJoin(serverURL string) tea.Cmd {
	room, nick, ok := m.prepIdentity()
	if !ok {
		return m.focusInput()
	}

	m.serverURL = serverURL
	m.busy = true
	m.busyText = "Joining room " + room + "..."
	m.errLine = ""
	m.blurHomeInputs()

	return joinCmd(serverURL, room, nick, strings.TrimSpace(m.pass.Value()), m.cfg.Color)
}

// joinRow fills the form from a discover row and joins it directly.
func (m *appModel) joinRow(row hubRow) tea.Cmd {
	m.room.SetValue(row.room)

	if row.online {
		return m.startJoin(m.serverURL)
	}

	return m.startJoin(fmt.Sprintf("ws://%s:%d/ws", row.addr, row.port))
}

// startHosting validates the form and boots the embedded LAN server; the
// hostedMsg handler joins it on localhost.
func (m *appModel) startHosting() tea.Cmd {
	_, _, ok := m.prepIdentity()
	if !ok {
		return m.focusInput()
	}

	m.busy = true
	m.busyText = fmt.Sprintf("Starting local server on :%d...", m.port)
	m.errLine = ""
	m.blurHomeInputs()

	return hostServerCmd(m.port, strings.TrimSpace(m.pass.Value()))
}

// onHosted joins the just-started local server and announces the beacon.
func (m appModel) onHosted() (tea.Model, tea.Cmd) {
	room := strings.TrimSpace(m.room.Value())
	nick := strings.TrimSpace(m.nick.Value())

	m.hostMode = true
	m.serverURL = fmt.Sprintf("ws://localhost:%d/ws", m.port)
	m.busyText = "Joining room " + room + " (self-hosted)..."

	startLANBroadcaster(room, m.port, nick)

	return m, joinCmd(m.serverURL, room, nick, strings.TrimSpace(m.pass.Value()), m.cfg.Color)
}

// enterChat hands the live connection to the chat model and switches the
// screen; the chat consumes the buffered join reply through waitForMessage.
func (m appModel) enterChat(conn *Connection) (tea.Model, tea.Cmd) {
	m.conn = conn
	m.busy = false

	room := strings.TrimSpace(m.room.Value())
	nick := strings.TrimSpace(m.nick.Value())

	chat := NewModel(conn, nick, room, m.theme)
	chat.serverURL = m.serverURL
	chat.color = m.cfg.Color
	chat.VoiceDevice = m.cfg.VoiceDevice

	if m.hostMode {
		chat.IsHost = true
		chat.HostIP = primaryLANIP()
		chat.HostPort = m.port
	}

	if line := shareLine(m); line != "" {
		appendUI(&chat, line)
	}

	if m.width > 0 && m.height > 0 {
		sized, _ := chat.Update(tea.WindowSizeMsg{Width: m.width, Height: m.height})
		chat = sized.(Model)
	}

	m.chat = chat
	m.screen = screenChat

	return m, m.chat.Init()
}

// shareLine tells the new chatter how peers can join: the one-liner URL for
// fresh cloud rooms, direct LAN instructions when self-hosting.
func shareLine(m appModel) string {
	room := strings.TrimSpace(m.room.Value())

	if m.hostMode {
		return fmt.Sprintf("Hosting room %s - peers join: termchat %s --host %s --port %d",
			room, room, primaryLANIP(), m.port)
	}

	if m.fresh {
		return fmt.Sprintf("Created room %s - share: %s/%s", room, m.base, room)
	}

	return ""
}

func (m appModel) View() string {
	if m.screen == screenChat {
		return m.chat.View()
	}

	return m.viewHome()
}

func (m appModel) viewHome() string {
	width := max(m.width, 20)

	rows := []string{
		m.viewHubHeader(width),
		m.hubRule(width),
		"",
		m.theme.system.Render("JOIN OR CREATE"),
	}
	rows = append(rows, m.viewForm()...)

	if line := m.viewStatus(); line != "" {
		rows = append(rows, "", line)
	}

	if m.showOnline {
		rows = append(rows, "")
		rows = append(rows, m.viewOnline(width)...)
	}

	if m.showLocal {
		rows = append(rows, "")
		rows = append(rows, m.viewLocal(width)...)
	}

	rows = append(rows, "", m.hubRule(width), m.viewFooter())

	for i := range rows {
		rows[i] = m.theme.base.Width(width).Render(rows[i])
	}

	return m.theme.base.
		Width(width).
		Height(max(m.height, 10)).
		Render(strings.Join(rows, "\n"))
}

// viewHubHeader renders the title bar: termchat left, version and theme
// right, padded to the full width.
func (m appModel) viewHubHeader(width int) string {
	left := m.theme.base.Bold(true).Render("termchat")
	right := m.theme.system.Render(hubVersion() + " - " + m.theme.Name)

	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)

	return restyleBareSpaces(m.theme, left+strings.Repeat(" ", gap)+right)
}

// hubVersion formats the build version for the header, e.g. "v2.3.0".
func hubVersion() string {
	if strings.HasPrefix(Version, "cli-") {
		return "v" + strings.TrimPrefix(Version, "cli-")
	}

	return Version
}

// hubRule is the flat dim divider between hub sections.
func (m appModel) hubRule(width int) string {
	return m.theme.system.Render(strings.Repeat("-", width))
}

func (m appModel) viewForm() []string {
	roomLine := restyleBareSpaces(m.theme, m.room.View())
	if m.fresh {
		roomLine += m.theme.system.Render(" (new room)")
	}

	return []string{
		m.hubField("Room", roomLine),
		m.hubField("Nickname", restyleBareSpaces(m.theme, m.nick.View())),
		m.hubField("Password", restyleBareSpaces(m.theme, m.pass.View())),
	}
}

// hubField pairs a dim fixed-width label with a field view.
func (m appModel) hubField(label, view string) string {
	return restyleBareSpaces(m.theme,
		m.theme.system.Render(fmt.Sprintf("%-10s", label))+view)
}

// viewStatus shows the spinner while busy, else the last error line. The
// busy text is painted explicitly: it trails the spinner's own reset.
func (m appModel) viewStatus() string {
	if m.busy {
		return m.spin.View() + m.theme.base.Render(" "+m.busyText)
	}

	if m.errLine != "" {
		return m.theme.system.Render("! " + m.errLine)
	}

	return ""
}

func (m appModel) viewOnline(width int) []string {
	header := "ONLINE ROOMS"
	if !m.onlineDone {
		header += " - scanning..."
	}

	lines := []string{m.theme.system.Render(header)}

	switch {
	case !m.onlineDone:
	case m.onlineErr != "":
		lines = append(lines, m.theme.system.Render("  "+firstLine(m.onlineErr)))
	default:
		lines = append(lines, m.hubRoomLines(true, width)...)
		if !m.hasOnlineRows() {
			lines = append(lines, m.theme.system.Render("  no rooms - join one above to create it"))
		}
	}

	return lines
}

func (m appModel) viewLocal(width int) []string {
	header := "LAN ROOMS"
	if !m.lanDone {
		header += " - scanning..."
	}

	lines := []string{m.theme.system.Render(header)}

	switch {
	case !m.lanDone:
	case !m.hasLocalRows():
		lines = append(lines, m.theme.system.Render("  none nearby - press h to host"))
	default:
		lines = append(lines, m.hubRoomLines(false, width)...)
	}

	return lines
}

func (m appModel) hasOnlineRows() bool {
	for _, r := range m.rows {
		if r.online {
			return true
		}
	}

	return false
}

func (m appModel) hasLocalRows() bool {
	for _, r := range m.rows {
		if !r.online {
			return true
		}
	}

	return false
}

// hubRoomLines renders one section of the combined cursor list; the cursor
// is live only while the list holds focus.
func (m appModel) hubRoomLines(online bool, width int) []string {
	var lines []string

	for i, r := range m.rows {
		if r.online != online {
			continue
		}

		left, meta := hubRowText(r)
		selected := m.focus == focusList && i == m.sel
		lines = append(lines, m.hubRowLine(left, meta, selected, width))
	}

	return lines
}

// hubRowText splits a discover row into its left cell and right meta. Only
// ASCII reaches the view: nicks are validated printable ASCII upstream.
func hubRowText(r hubRow) (string, string) {
	host := r.host
	if host == "" {
		host = "-"
	}

	if len(host) > 16 {
		host = host[:16]
	}

	if r.online {
		left := r.room
		if r.locked {
			left += " [locked]"
		}

		return left, fmt.Sprintf("%d online - host %s", r.users, host)
	}

	return r.room, fmt.Sprintf("host %s - %s:%d", host, r.addr, r.port)
}

// hubRowLine renders one discover row with right-aligned meta. The selected
// row is a full-width bar with unstyled inner content, so no reset can cut
// the bar's background short.
func (m appModel) hubRowLine(left, meta string, selected bool, width int) string {
	cursor := "  "
	if selected {
		cursor = "> "
	}

	plain := cursor + left
	gap := max(width-len(plain)-len(meta), 1)
	line := plain + strings.Repeat(" ", gap) + meta

	if selected {
		return m.theme.completionSelected.Width(width).Render(line)
	}

	return restyleBareSpaces(m.theme, plain+strings.Repeat(" ", gap)+m.theme.system.Render(meta))
}

func (m appModel) viewFooter() string {
	return m.theme.system.Render("enter join - tab focus - h host - ctrl+r rescan - esc quit")
}

// firstLine keeps multi-line fetch errors to one hub status line.
func firstLine(s string) string {
	if idx := strings.Index(s, "\n"); idx >= 0 {
		return s[:idx]
	}

	return s
}

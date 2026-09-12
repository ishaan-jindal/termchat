package main

import (
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
)

// bareSpaceRe matches plain spaces directly after a reset sequence.
var bareSpaceRe = regexp.MustCompile("\x1b\\[0m +")

// restyleBareSpaces gives those spaces the theme background: the textarea
// leaves some lines short of the input width and its internal viewport pads
// the remainder unstyled.
func restyleBareSpaces(theme Theme, s string) string {
	return bareSpaceRe.ReplaceAllStringFunc(s, func(match string) string {
		return "\x1b[0m" + theme.base.Render(match[4:])
	})
}

// lineKind selects how a chatLine re-renders under a new theme.
type lineKind int

const (
	lineChat   lineKind = iota // user message
	lineSystem                 // server event, "[system]" prefix
	lineUI                     // local feedback, no prefix
)

// chatLine holds one rendered chat message; the raw Message is kept so the
// line can be re-rendered when its reactions or the theme change.
type chatLine struct {
	kind     lineKind
	msg      Message
	rendered string
}

// IncomingMessage is one server frame delivered to the TUI.
type IncomingMessage Message

// Model is the chat screen state.
type Model struct {
	conn *Connection

	theme Theme

	messages []chatLine
	input    textarea.Model

	// msgIndex maps a message ID to its index in messages.
	msgIndex map[int64]int

	nick      string
	room      string
	users     []UserInfo
	connected bool

	// serverURL, color, and conn.password carry the session credentials
	// needed to rejoin after a transient network drop.
	serverURL string
	color     string

	IsHost   bool
	HostIP   string
	HostPort int

	viewport viewport.Model
	width    int
	height   int

	autoScroll bool

	compactMode bool
	showSidebar bool

	history      []string
	historyIndex int

	lastTypingSent time.Time

	// usersRequested makes the next users_list print into the chat log.
	usersRequested bool

	// clockOffset is server_time minus local time from the latest
	// users_list; relative times are rendered on the server's timeline.
	clockOffset int64

	showPopup bool
	selected  int

	voice *VoiceSession
	video *VideoSession

	// media is the single shared /media WebSocket for voice and video;
	// the sessions above only borrow it, the Model owns its lifecycle.
	media *MediaConn

	// wantVoice and wantVideo record which sessions a media_token request
	// was issued for; mediaReadyMsg attaches the marked ones.
	wantVoice bool
	wantVideo bool

	// videoColVisible remembers whether the sidebar video column was last
	// shown, so the video tick can refit the layout on transitions.
	videoColVisible bool

	// VoiceDevice is the configured microphone name passed to ffmpeg.
	VoiceDevice string

	// CameraDevice is the configured webcam path passed to ffmpeg.
	CameraDevice string

	// tokenPending guards against duplicate media requests while the
	// media_token reply or its timeout tick is still in flight.
	tokenPending bool

	// pendingCmd carries a tea.Cmd out of a slash-command handler; Update
	// returns it alongside the handled model.
	pendingCmd tea.Cmd
}

// NewModel builds the chat screen for one joined room.
func NewModel(conn *Connection, nick string, room string, theme Theme) Model {
	ti := textarea.New()

	ti.Placeholder = "Type a message..."
	ti.Focus()

	ti.ShowLineNumbers = false
	ti.SetHeight(3)
	ti.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(0, 0)

	m := Model{
		conn:         conn,
		theme:        theme,
		messages:     []chatLine{},
		msgIndex:     map[int64]int{},
		input:        ti,
		nick:         nick,
		room:         room,
		users:        []UserInfo{},
		connected:    true,
		viewport:     vp,
		history:      []string{},
		historyIndex: 0,
		autoScroll:   true,
	}

	m.applyInputStyles()

	return m
}

// applyInputStyles wires the active theme into the textarea and cursor.
// The textarea keeps an internal pointer to its focused/blurred style
// captured by Focus/Blur, so it must be re-seated after the assignment.
func (m *Model) applyInputStyles() {
	st := m.theme.input
	st.Prompt = m.theme.accent
	m.input.FocusedStyle = st
	m.input.BlurredStyle = st
	m.input.Prompt = "> "
	m.input.Cursor.Style = m.theme.cursor
	m.input.Cursor.TextStyle = m.theme.input.Text

	focused := m.input.Focused()
	m.input.Blur()
	m.input.Focus()

	if !focused {
		m.input.Blur()
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

		case "ctrl+t":
			cmd := toggleTalk(&m)

			return m, cmd

		case "ctrl+v":
			cmd := toggleVideo(&m)

			pending := m.pendingCmd
			m.pendingCmd = nil

			return m, tea.Batch(cmd, pending)

		case "pgup", "pgdown":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.autoScroll = m.viewport.AtBottom()
			return m, cmd

		case "tab":
			if m.showPopup {
				acceptCompletion(&m)
				return m, nil
			}

			m.showPopup = true
			refreshCompletion(&m)

			return m, nil

		case "esc":
			dismissCompletion(&m)

			return m, nil

		case "up":
			if m.showPopup {
				m.selected = max(m.selected-1, 0)
				return m, nil
			}

			if m.input.Line() == 0 {
				if len(m.history) > 0 && m.historyIndex > 0 {
					m.historyIndex--
					m.input.SetValue(m.history[m.historyIndex])
				}
				return m, nil
			}

		case "down":
			if m.showPopup {
				if n := len(completionMatches(&m)); n > 0 {
					m.selected = min(m.selected+1, n-1)
				}
				return m, nil
			}

			totalLines := strings.Count(m.input.Value(), "\n") + 1
			if m.input.Line() >= totalLines-1 {
				if len(m.history) > 0 && m.historyIndex < len(m.history)-1 {
					m.historyIndex++
					m.input.SetValue(m.history[m.historyIndex])
				} else {
					m.historyIndex = len(m.history)
					m.input.SetValue("")
				}
				return m, nil
			}

		case "alt+enter":
			m.input.InsertRune('\n')
			refreshCompletion(&m)
			return m, nil

		case "enter":
			if m.showPopup {
				acceptCompletion(&m)
				return m, nil
			}

			text := strings.TrimSpace(m.input.Value())
			if strings.HasPrefix(text, "/") {
				before := m.theme.Name
				handled, quit := handleCommand(&m, text)
				if handled {
					m.input.Reset()

					cmd := m.pendingCmd
					m.pendingCmd = nil

					if quit {
						return m, tea.Quit
					}

					// A theme change must repaint the whole frame:
					// the renderer's line diff can leave stale cells.
					if m.theme.Name != before {
						return m, tea.ClearScreen
					}

					return m, cmd
				}
			}
			if text != "" {
				m.history = append(m.history, text)
				m.historyIndex = len(m.history)

				trySend(&m, Message{
					Type: "message",
					Text: text,
				})
				m.input.Reset()
			}
			return m, nil
		}

		var cmd tea.Cmd

		previousValue := m.input.Value()
		m.input, cmd = m.input.Update(msg)

		if m.input.Value() != previousValue {
			refreshCompletion(&m)

			if time.Since(m.lastTypingSent) > 2*time.Second {
				trySend(&m, Message{Type: "typing"})
				m.lastTypingSent = time.Now()
			}
		}

		return m, cmd

	case tea.MouseMsg:
		switch msg.Button {

		case tea.MouseButtonWheelUp:
			m.viewport.ScrollUp(3)

		case tea.MouseButtonWheelDown:
			m.viewport.ScrollDown(3)
		}

		m.autoScroll = m.viewport.AtBottom()

		return m, nil

	case IncomingMessage:

		switch msg.Type {

		case "system", "message":
			appendFormattedMessage(&m, Message(msg))

		case "reaction":
			idx, ok := m.msgIndex[msg.ID]
			if ok {
				target := m.messages[idx].msg

				// A growing vote total means a reaction was added,
				// not removed.
				before, after := 0, 0
				for _, r := range target.Reactions {
					before += r.Count
				}
				for _, r := range msg.Reactions {
					after += r.Count
				}

				if target.Nick == m.nick && msg.Nick != "" && msg.Nick != m.nick && after > before {
					print("\a")
					notify(fmt.Sprintf("%s reacted to your message", msg.Nick), target.Text)
				}

				m.messages[idx].msg.Reactions = msg.Reactions
				m.messages[idx].rendered = paintLine(&m, renderMessage(&m, m.messages[idx].msg))
			}

		case "users_list":
			m.users = msg.Users

			if msg.ServerTime != 0 {
				m.clockOffset = msg.ServerTime - time.Now().Unix()
			}

			if m.usersRequested {
				appendUsersList(&m)
				m.usersRequested = false
			}

		case "media_token":
			if m.voice == nil && m.tokenPending && msg.Token != "" {
				m.tokenPending = false
				m.pendingCmd = dialMediaCmd(m.conn.base, m.room, msg.Token)
			}

		case "history":
			for _, historyMsg := range msg.Messages {
				appendFormattedMessage(&m, historyMsg)
			}
		}

		wasAtBottom := m.autoScroll || m.viewport.AtBottom()

		m.viewport.SetContent(strings.Join(renderedLines(&m), "\n"))

		if wasAtBottom {
			m.viewport.GotoBottom()
		}

		cmd := m.pendingCmd
		m.pendingCmd = nil

		return m, tea.Batch(waitForMessage(m.conn), cmd)

	case connErrMsg:
		if m.conn != nil {
			select {
			case <-m.conn.done:
				// writePump already stopped
			default:
				close(m.conn.done)
			}

			if m.conn.conn != nil {
				m.conn.conn.Close()
			}
		}

		password := ""
		if m.conn != nil {
			password = m.conn.password
		}

		m.connected = false

		if m.voice != nil {
			m.voice.Shutdown()
			m.voice = nil
			appendUI(&m, "voice session ended: connection lost")
		}

		if m.video != nil {
			m.video.Shutdown()
			m.video = nil
			m.videoColVisible = false
			appendUI(&m, "video session ended: connection lost")
		}

		m.closeMediaIfIdle()

		appendUI(&m, "connection lost, reconnecting...")

		return m, reconnectCmd(m.serverURL, m.room, m.nick, password, m.color)

	case reconnectedMsg:
		m.conn = msg.conn
		m.connected = true

		appendUI(&m, "reconnected")

		return m, waitForMessage(m.conn)

	case connFatalMsg:
		m.connected = false

		appendUI(&m, "reconnect failed: "+msg.err.Error())

		return m, tea.Quit

	case mediaReadyMsg:
		m.media = msg.conn

		cmds := []tea.Cmd{waitForMediaEnd(msg.conn)}

		if m.wantVoice && m.voice == nil {
			cmds = append(cmds, m.startVoiceSession())
		}

		if m.wantVideo && m.video == nil {
			cmds = append(cmds, m.startVideoSession())
		}

		m.wantVoice = false
		m.wantVideo = false

		return m, tea.Batch(cmds...)

	case mediaErrorMsg:
		m.tokenPending = false
		m.wantVoice = false
		m.wantVideo = false
		appendUI(&m, "media unavailable: "+msg.err.Error())

		return m, nil

	case mediaEndedMsg:
		if m.voice != nil {
			m.voice.Shutdown()
			m.voice = nil
			appendUI(&m, "voice session ended")
		}

		if m.video != nil {
			m.video.Shutdown()
			m.video = nil
			m.videoColVisible = false
			appendUI(&m, "video session ended")
		}

		m.closeMediaIfIdle()
		refitLayout(&m)

		return m, nil

	case voicePlaybackStoppedMsg:
		if m.voice != nil && m.voice.play == msg.play {
			tail := msg.tail
			if tail == "" {
				tail = "unknown reason"
			}

			m.voice.Shutdown()
			m.voice = nil
			appendUI(&m, "playback stopped: "+tail)
		}

		return m, nil

	case voiceActivityTickMsg:
		if m.voice != nil {
			return m, voiceActivityTicker()
		}

		return m, nil

	case voiceMicStoppedMsg:
		if m.voice != nil && m.voice.tx && m.voice.mic == msg.mic {
			m.voice.tx = false
			m.voice.mic = nil

			text := "microphone stopped unexpectedly"
			if msg.tail != "" {
				text += ": " + msg.tail
			}

			appendUI(&m, text)
		}

		return m, nil

	case mediaTimeoutTickMsg:
		if m.tokenPending {
			m.tokenPending = false
			m.wantVoice = false
			m.wantVideo = false
			appendUI(&m, "server did not answer the media request; it may be too old for vc")
		}

		return m, nil

	case videoTickMsg:
		if m.video != nil {
			m.video.prune()

			vis := m.showSidebar && m.videoColumnVisible()

			if vis != m.videoColVisible {
				m.videoColVisible = vis
				refitLayout(&m)
			}

			return m, videoTicker()
		}

		return m, nil

	case videoCamStoppedMsg:
		if m.video != nil && m.video.tx && m.video.cam == msg.cam {
			m.video.tx = false
			m.video.cam = nil

			text := "camera stopped unexpectedly"
			if msg.tail != "" {
				text += ": " + msg.tail
			}

			appendUI(&m, text)
		}

		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		fitWidths(&m)
		resizeViewport(&m)
		rerenderAll(&m)

		return m, nil
	}

	var cmd tea.Cmd

	m.input, cmd = m.input.Update(msg)
	m.input.SetHeight(textareaHeight(m.input))

	return m, cmd
}

func (m Model) View() string {
	scrollInfo := ""

	if !m.viewport.AtTop() {
		scrollInfo += "^"
	}

	if !m.viewport.AtBottom() {
		scrollInfo += "v"
	}

	msgView := m.viewport
	msgView.Height = max(m.viewport.Height-1, 1)

	messagesPanel := m.theme.panel.
		Width(m.viewport.Width + 4).
		Height(m.viewport.Height).
		Render(m.messagesHeader(scrollInfo) + "\n" + msgView.View())

	var content string

	if m.showSidebar {
		content = lipgloss.JoinHorizontal(
			lipgloss.Top,
			messagesPanel,
			renderSidebar(m),
		)
	} else {
		content = messagesPanel
	}

	input := m.theme.panel.
		Width(m.width - 6).
		Render(restyleBareSpaces(m.theme, m.input.View()))

	voiceInfo := ""

	if m.voice != nil {
		now := time.Now().UnixMilli()

		sending := m.voice.tx &&
			now-m.voice.lastSent.Load() < 600
		hearing := now-m.voice.lastRecv.Load() < 600

		voiceInfo = " - VOICE"

		if sending {
			voiceInfo += " [TX*]"
		} else if m.voice.tx {
			voiceInfo += " [TX]"
		}

		if hearing {
			voiceInfo += " [RX*]"
		}
	}

	videoInfo := ""

	if m.video != nil {
		now := time.Now().UnixMilli()

		sending := m.video.tx &&
			now-m.video.lastSent.Load() < 600
		hearing := now-m.video.lastRecv.Load() < 600

		videoInfo = " - VIDEO"

		if sending {
			videoInfo += " [TX*]"
		} else if m.video.tx {
			videoInfo += " [TX]"
		}

		if hearing {
			videoInfo += " [RX*]"
		}
	}

	onVideo := len(m.activeVideoTiles())
	onVideoInfo := ""
	if onVideo > 0 {
		onVideoInfo = fmt.Sprintf(" - %d on video", onVideo)
	}

	state := "Connected"
	if !m.connected {
		state = "Reconnecting"
	}

	roomInfo := fmt.Sprintf("%s - Room %s - %d users", state, m.room, len(m.users))

	if m.IsHost {
		roomInfo = fmt.Sprintf(
			"SELF-HOSTED - Room %s - %s:%d - %d users",
			m.room,
			m.HostIP,
			m.HostPort,
			len(m.users),
		)

		if !m.connected {
			roomInfo = fmt.Sprintf(
				"RECONNECTING - Room %s - %s:%d",
				m.room,
				m.HostIP,
				m.HostPort,
			)
		}
	}

	status := m.renderStatusBar(roomInfo, voiceInfo+videoInfo+onVideoInfo)

	rows := []string{content}

	if m.showPopup {
		if popup := renderCompletion(m); popup != "" {
			rows = append(rows, popup)
		}
	}

	if m.videoPanelHeight() > 0 {
		rows = append(rows, m.renderVideoPanel())
	}

	rows = append(rows, input, status)

	// Every row is painted to the full terminal width so JoinVertical
	// never inserts plain unstyled padding between blocks.
	for i := range rows {
		rows[i] = m.theme.base.Width(m.width).Render(rows[i])
	}

	ui := lipgloss.JoinVertical(
		lipgloss.Left,
		rows...,
	)

	// Joins pad with plain spaces, so repaint stragglers with the theme
	// background before the canvas goes on.
	ui = restyleBareSpaces(m.theme, ui)

	// The canvas paints every remaining cell (join gaps, panel margins,
	// filler lines) so a named theme fully covers the terminal colors.
	return m.theme.base.
		Width(m.width).
		Height(m.height).
		Render(ui)
}

// renderVideoPanel renders the embedded video grid in a bordered panel the
// same width as the messages panel.
func (m Model) renderVideoPanel() string {
	inner := m.viewport.Width
	contentH := m.videoPanelHeight() - 2

	tiles := m.activeVideoTiles()

	var lines []string

	if len(tiles) > 0 {
		lines = renderVideoTiles(tiles, inner, contentH, m.theme)
	} else {
		lines = blankLines(inner, contentH)
		lines[0] = tileCaption("starting camera...", inner)
	}

	return m.theme.panel.
		Width(m.viewport.Width + 4).
		Height(m.videoPanelHeight()).
		Render(strings.Join(lines, "\n"))
}

type videoPeer struct {
	id    uint32
	frame *videoPeerFrame
}

// renderStatusBar composes the segmented footer: an accent-dotted state
// cluster left, dim keycap hints right. Hints shrink away on narrow
// terminals rather than wrapping the bar.
func (m Model) renderStatusBar(roomInfo, badges string) string {
	inner := max(m.width-10, 10)

	dot := m.theme.accent.Render("o")
	if !m.connected {
		dot = m.theme.hint.Render("x")
	}

	// Every literal lives inside a span so no cell is left to the terminal
	// background. The inner style is padding-free: the outer bar applies
	// the only padding, or nested padding would push the line over budget.
	st := m.theme.status.Padding(0)
	left := dot + st.Render(" "+roomInfo+badges)

	var hints []string

	if inVC(&m) {
		hints = append(hints,
			m.theme.statusKey.Render("ctrl+t")+m.theme.statusHint.Render(" mic"),
			m.theme.statusKey.Render("ctrl+v")+m.theme.statusHint.Render(" cam"),
		)
	}

	line := left

	if len(hints) > 0 {
		right := strings.Join(hints, m.theme.statusHint.Render(" | "))

		if w := lipgloss.Width(left) + 2 + lipgloss.Width(right); w <= inner {
			line += st.Render(strings.Repeat(" ", inner-lipgloss.Width(left)-lipgloss.Width(right))) + right
		}
	}

	return m.theme.panel.
		Width(m.width - 6).
		Render(
			m.theme.status.
				Width(m.width - 8).
				Render(line),
		)
}

// messagesHeader renders the slim title bar atop the message panel: the
// room and roster size left, scroll indicators right.
func (m Model) messagesHeader(scrollInfo string) string {
	left := fmt.Sprintf("ROOM %s - %d users", m.room, len(m.users))

	inner := max(m.viewport.Width, 10)
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(scrollInfo), 1)
	line := left + strings.Repeat(" ", gap) + scrollInfo

	return m.theme.hint.Render(line)
}

// activeVideoTiles snapshots the local preview plus the freshest peer
// streams, resolving sender nicks from the room roster.
func (m *Model) activeVideoTiles() []videoTile {
	s := m.video
	if s == nil {
		return nil
	}

	cutoff := time.Now().Add(-videoStaleAfter)

	s.mu.Lock()
	self := s.self

	var peers []videoPeer

	for id, f := range s.peers {
		if f.updated.Before(cutoff) {
			continue
		}

		peers = append(peers, videoPeer{id: id, frame: f})
	}

	s.mu.Unlock()

	sort.Slice(peers, func(i, j int) bool {
		return peers[i].frame.updated.After(peers[j].frame.updated)
	})

	var tiles []videoTile

	if self != nil && s.tx {
		tiles = append(tiles, videoTile{nick: m.nick, color: m.color, tag: "(you)", pix: self.pix, w: self.w, h: self.h, self: true})
	}

	for _, p := range peers {
		if len(tiles) >= maxVideoTiles {
			break
		}

		nick, color, tag := m.videoTileLabel(p.id)
		tiles = append(tiles, videoTile{nick: nick, color: color, tag: tag, pix: p.frame.pix, w: p.frame.w, h: p.frame.h})
	}

	return tiles
}

// videoTileLabel resolves a stream to its display parts: plain nick, roster
// color and trailing tag. Unknown streams fall back to a cam-N label.
func (m *Model) videoTileLabel(id uint32) (nick, color, tag string) {
	for _, u := range m.users {
		if u.VoiceID != id {
			continue
		}

		if u.IsHost {
			return u.Nick, u.Color, "[host]"
		}

		return u.Nick, u.Color, ""
	}

	return fmt.Sprintf("cam-%d", id), "", ""
}

// videoNick resolves a stream ID to a roster nick via the ID the server
// stamped on relayed frames.
func (m *Model) videoNick(id uint32) string {
	nick, _, _ := m.videoTileLabel(id)

	return nick
}

// videoCamNicks lists roster nicks with a live stream, for the sidebar.
func (m *Model) videoCamNicks() []string {
	s := m.video
	if s == nil {
		return nil
	}

	cutoff := time.Now().Add(-videoStaleAfter)

	s.mu.Lock()
	defer s.mu.Unlock()

	var out []string

	if s.self != nil && s.tx {
		out = append(out, m.nick)
	}

	for id, f := range s.peers {
		if f.updated.Before(cutoff) {
			continue
		}

		out = append(out, m.videoNick(id))
	}

	return out
}

// videoPanelHeight is the embedded panel's total height including its
// border, or zero when the panel stays hidden. The sidebar replaces the
// bottom panel whenever it is visible.
func (m *Model) videoPanelHeight() int {
	if m.video == nil || m.height < 20 || m.viewport.Width < 20 {
		return 0
	}

	if m.showSidebar {
		return 0
	}

	if len(m.activeVideoTiles()) == 0 && !m.video.tx {
		return 0
	}

	content := m.height / 5
	if content < 6 {
		content = 6
	}
	if content > 12 {
		content = 12
	}

	return content + 2
}

// sectionHeader renders a centered sidebar header.
func sectionHeader(m Model, label string, inner int) string {
	return m.theme.usersHeader.Width(inner).Align(lipgloss.Center).Render(label)
}

// renderSidebar paints the right column: the video tiles above the users
// roster, in one panel sized to the message viewport.
func renderSidebar(m Model) string {
	// The panel's border and padding add 2 cells to whatever Width is
	// requested, so back out 2 to land exactly on the reserved sidebar width.
	panelW := m.sidebarWidth() - 2
	inner := panelW - 2

	camSet := map[string]bool{}
	for _, nick := range m.videoCamNicks() {
		camSet[nick] = true
	}

	lines := renderRoster(m, inner, camSet)

	// Roster must keep a floor: header, rule, blank and two nick rows.
	rosterFloor := 5
	if m.showSidebar && m.video != nil {
		lines = renderVideoSection(m, inner, rosterFloor, lines)
	}

	content := strings.Join(lines, "\n")
	return m.theme.panel.
		Width(panelW).
		Height(m.viewport.Height).
		Render(content)
}

// presenceTags returns the shared host and voice markers for a user.
func presenceTags(user UserInfo) string {
	tags := ""
	if user.IsHost {
		tags += "[host] "
	}
	if user.VoiceID != 0 {
		tags += "[VC] "
	}

	return tags
}

// rosterRow renders one nick line.
func rosterRow(m Model, user UserInfo, inner int, camSet map[string]bool) string {
	nick := user.Nick
	if m.compactMode && len(nick) > 8 {
		nick = nick[:8]
	}

	joined := relativeTime(user.JoinedAt - m.clockOffset)
	status := presenceTags(user)
	if camSet[user.Nick] {
		status += "[CAM] "
	}
	if user.Typing {
		status += "[...] "
	}

	coloredNick := lipgloss.NewStyle().
		Foreground(lipgloss.Color(user.Color)).
		Background(m.theme.base.GetBackground()).
		Bold(true).
		Render("* " + nick)

	line := coloredNick + m.theme.base.Render(fmt.Sprintf("%4s %s", joined, status))

	return ansi.Truncate(line, inner, "")
}

// renderRoster builds the USERS section: header, rule, blank, then nick
// rows. It returns the rows so the video column can borrow height above.
func renderRoster(m Model, inner int, camSet map[string]bool) []string {
	var lines []string

	lines = append(lines, sectionHeader(m, "USERS", inner))
	lines = append(lines, strings.Repeat("-", inner))
	lines = append(lines, "")

	for _, u := range m.users {
		lines = append(lines, rosterRow(m, u, inner, camSet))
	}

	return lines
}

// renderVideoSection prepends the video column above the roster: a VIDEO
// header, bordered tiles (accent for self) and a +N overflow line,
// stopping short of the roster floor. An idle call hides the section;
// a transmitting-but-frameless call shows a slim placeholder instead.
func renderVideoSection(m Model, inner, rosterFloor int, roster []string) []string {
	tiles := m.activeVideoTiles()

	if len(tiles) == 0 && (m.video == nil || !m.video.tx) {
		return roster
	}

	var section []string

	section = append(section, sectionHeader(m, "VIDEO", inner))
	section = append(section, strings.Repeat("-", inner))

	if len(tiles) == 0 {
		section = append(section, m.theme.system.Render(tileCaption("starting camera...", inner)))

		return append(section, roster...)
	}

	avail := m.viewport.Height - len(roster)
	if avail < rosterFloor {
		avail = rosterFloor
	}
	avail -= len(section)

	tileH := sidebarTileVidH + 1 // caption + video lines
	tileBox := func(accent bool) lipgloss.Style {
		style := m.theme.panel.Inherit(m.theme.panel).Padding(0).
			BorderBackground(lipgloss.Color(m.theme.bgHex))

		if accent {
			return style.BorderForeground(lipgloss.Color(m.theme.accentHex))
		}

		return style.
			BorderForeground(lipgloss.Color(m.theme.borderDimHex))
	}

	// The nested box must stay within the outer panel's wrap budget
	// (width - 2); the box's own border consumes 2 more cells.
	tileW := inner - 2

	shown := 0

	for _, tile := range tiles {
		if len(section)+tileH > avail {
			break
		}

		if tileW < 6 {
			break
		}

		body := renderTile(tile, tileW, tileH, m.theme)
		box := tileBox(tile.self).Width(tileW).Render(strings.Join(body, "\n"))
		section = append(section, strings.Split(box, "\n")...)
		shown++
	}

	if shown < len(tiles) {
		section = append(section, m.theme.system.Render(
			fmt.Sprintf("+%d more on video", len(tiles)-shown)))
	}

	return append(section, roster...)
}

func appendFormattedMessage(m *Model, msg Message) {
	switch msg.Type {

	case "system":
		appendLine(m, chatLine{kind: lineSystem, msg: msg})

	case "message":
		appendLine(m, chatLine{kind: lineChat, msg: msg})

		title := ""

		switch {
		case isMention(msg, m.nick):
			title = fmt.Sprintf("%s mentioned you", msg.Nick)

		case msg.ReplyToID != 0 && msg.ReplyToNick == m.nick && msg.Nick != m.nick:
			title = fmt.Sprintf("%s replied to your message", msg.Nick)
		}

		if title != "" {
			print("\a")
			notify(title, msg.Text)
		}

		if msg.ID != 0 {
			m.msgIndex[msg.ID] = len(m.messages) - 1
		}
	}
}

// appendLine renders a line under the current theme and stores it.
func appendLine(m *Model, line chatLine) {
	line.rendered = paintLine(m, renderChatLine(m, line))
	m.messages = append(m.messages, line)
}

// paintLine pads a rendered line to the viewport width with the theme
// background, so the viewport's own unstyled padding never shows.
func paintLine(m *Model, s string) string {
	if m.viewport.Width <= 0 {
		return s
	}

	return m.theme.base.Width(m.viewport.Width).Render(s)
}

// appendUI appends a local feedback line without the [system] prefix.
func appendUI(m *Model, text string) {
	appendLine(m, chatLine{kind: lineUI, msg: Message{Text: text}})

	m.viewport.SetContent(strings.Join(renderedLines(m), "\n"))
	m.viewport.GotoBottom()
}

// renderChatLine dispatches a line to its renderer under the active theme.
func renderChatLine(m *Model, line chatLine) string {
	switch line.kind {
	case lineSystem:
		return renderSystemMessage(m, line.msg)
	case lineChat:
		return renderMessage(m, line.msg)
	default:
		return renderUILine(m, line.msg)
	}
}

// rerenderAll rebuilds every cached line, e.g. after a theme switch or a
// terminal resize.
func rerenderAll(m *Model) {
	for i := range m.messages {
		m.messages[i].rendered = paintLine(m, renderChatLine(m, m.messages[i]))
	}

	m.viewport.SetContent(strings.Join(renderedLines(m), "\n"))
}

func renderedLines(m *Model) []string {
	lines := make([]string, 0, len(m.messages))

	for _, line := range m.messages {
		lines = append(lines, line.rendered)
	}

	return lines
}

func isMention(msg Message, nick string) bool {
	return strings.Contains(
		strings.ToLower(msg.Text),
		"@"+strings.ToLower(nick),
	)
}

// renderUILine renders a local feedback line, dim and without a prefix.
func renderUILine(m *Model, msg Message) string {
	plain := msg.Text

	if m.viewport.Width > 0 && runtime.GOARCH != "386" {
		plain = wordwrap.String(plain, m.viewport.Width)
	}

	return m.theme.system.Render(plain)
}

func renderSystemMessage(m *Model, msg Message) string {
	plain := "[system] " + msg.Text

	if m.viewport.Width > 0 && runtime.GOARCH != "386" {
		plain = wordwrap.String(plain, m.viewport.Width)
	}

	return m.theme.system.Render(plain)
}

func renderMessage(m *Model, msg Message) string {
	mentioned := isMention(msg, m.nick)

	idPrefix := ""
	if msg.ID != 0 {
		idPrefix = fmt.Sprintf("#%d ", msg.ID)
	}

	// The nick span follows the id prefix's reset, so it must carry the
	// theme background itself or the terminal's bleeds through.
	nickStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(msg.Color)).
		Background(m.theme.base.GetBackground()).
		Bold(true)

	prefix := idPrefix + msg.Nick + ": "
	availableWidth := max(m.viewport.Width-len(prefix), 10)
	wrapped := msg.Text
	if runtime.GOARCH != "386" {
		wrapped = wordwrap.String(msg.Text, availableWidth)
	}
	lines := strings.Split(wrapped, "\n")

	if mentioned {
		nickStyle = nickStyle.Background(m.theme.mention.GetBackground())
		for i := range lines {
			if i == 0 {
				lines[i] = m.theme.system.Render(idPrefix) + nickStyle.Render(msg.Nick) + m.theme.mention.Render(": "+lines[i])
			} else {
				lines[i] = m.theme.mention.Render(strings.Repeat(" ", len(prefix)) + lines[i])
			}
		}
	} else {
		renderedNick := nickStyle.Render(msg.Nick)
		for i := range lines {
			if i == 0 {
				// Plain segments are themed explicitly: the reset at
				// the end of a colored span would otherwise drop the
				// background for the rest of the line.
				lines[i] = m.theme.system.Render(idPrefix) + renderedNick + m.theme.base.Render(": "+lines[i])
			} else {
				lines[i] = m.theme.base.Render(strings.Repeat(" ", len(prefix)) + lines[i])
			}
		}
	}

	var out []string

	if quote := formatQuote(m, msg, availableWidth); quote != "" {
		out = append(out, quote)
	}

	out = append(out, lines...)

	if len(msg.Reactions) > 0 {
		last := len(out) - 1
		out[last] += m.theme.system.Render("  " + formatReactions(msg.Reactions))
	}

	return strings.Join(out, "\n")
}

// formatQuote renders the quoted message of a reply as a dim quote line, or
// an empty string when the message is not a reply.
func formatQuote(m *Model, msg Message, width int) string {
	if msg.ReplyToID == 0 {
		return ""
	}

	quote := "> " + msg.ReplyToNick + ": " + msg.ReplyToText

	if width > 0 && runtime.GOARCH != "386" {
		quote = wordwrap.String(quote, width)
	}

	return m.theme.system.Render(quote)
}

// formatReactions renders reaction counts as inline bracketed glyphs.
func formatReactions(reactions []Reaction) string {
	if len(reactions) == 0 {
		return ""
	}

	parts := make([]string, 0, len(reactions))

	for _, r := range reactions {
		glyph := emojiGlyph(r.Name)

		if glyph == "" {
			glyph = r.Name
		}

		if r.Count > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", glyph, r.Count))
		} else {
			parts = append(parts, glyph)
		}
	}

	return "[" + strings.Join(parts, ", ") + "]"
}

// appendUsersList writes the current room roster into the chat log.
func appendUsersList(m *Model) {
	appendFormattedMessage(m, Message{
		Type: "system",
		Text: fmt.Sprintf("Users (%d):", len(m.users)),
	})

	for _, user := range m.users {
		line := "  " + user.Nick
		if tags := strings.TrimSpace(presenceTags(user)); tags != "" {
			line += " " + tags
		}

		appendFormattedMessage(m, Message{
			Type: "system",
			Text: line,
		})
	}
}

// completionMatches returns the suggestions for the current input, or nil
// when the popup is closed or nothing matches.
func completionMatches(m *Model) []suggestion {
	if !m.showPopup {
		return nil
	}

	matches, _ := matchSuggestions(m.input.Value(), m.users, m.nick)

	return matches
}

// refreshCompletion reopens or refilters the popup from the current input.
func refreshCompletion(m *Model) {
	matches, _ := matchSuggestions(m.input.Value(), m.users, m.nick)

	open := len(matches) > 0

	if open && m.selected >= len(matches) {
		m.selected = len(matches) - 1
	}

	if !open {
		m.selected = 0
	}

	m.showPopup = open

	resizeViewport(m)
}

func dismissCompletion(m *Model) {
	if !m.showPopup {
		return
	}

	m.showPopup = false
	m.selected = 0

	resizeViewport(m)
}

// acceptCompletion inserts the selected suggestion in place of the current
// token and closes the popup.
func acceptCompletion(m *Model) {
	matches, tokenLen := matchSuggestions(m.input.Value(), m.users, m.nick)

	m.showPopup = false

	if len(matches) == 0 {
		return
	}

	m.selected = min(m.selected, len(matches)-1)

	value := m.input.Value()
	m.input.SetValue(value[:len(value)-tokenLen] + matches[m.selected].insert)
	m.input.CursorEnd()

	resizeViewport(m)
}

// resizeViewport refits the viewport height from the terminal size, the
// input height and the popup height.
func resizeViewport(m *Model) {
	popupHeight := len(completionMatches(m))
	if popupHeight > 0 {
		popupHeight += 2 // rounded border
	}

	m.viewport.Height = max(
		m.height-textareaHeight(m.input)-popupHeight-7-m.videoPanelHeight(),
		5,
	)
}

func renderCompletion(m Model) string {
	matches := completionMatches(&m)
	if len(matches) == 0 {
		return ""
	}

	sel := min(m.selected, len(matches)-1)

	width := 0

	for _, s := range matches {
		width = max(width, len(s.primary))
	}

	rows := make([]string, 0, len(matches))

	for i, s := range matches {
		// Each column is rendered with one style so the selected row's
		// background is not cut short by an inner reset sequence.
		rowStyle := m.theme.base

		if i == sel {
			rowStyle = m.theme.completionSelected
		}

		primaryStyle := rowStyle
		if s.primaryStyle != nil {
			primaryStyle = s.primaryStyle.
				Background(m.theme.base.GetBackground())
		}

		rows = append(rows,
			primaryStyle.Render(fmt.Sprintf("%-*s", width, s.primary)),
		)
	}

	return m.theme.panel.
		Width(m.width - 6).
		Render(strings.Join(rows, "\n"))
}

func textareaHeight(input textarea.Model) int {
	lines := strings.Count(input.Value(), "\n") + 1
	if lines < 3 {
		return 3
	}
	if lines > 8 {
		return 8
	}
	return lines
}

// videoColumnVisible reports whether the sidebar carries the video column:
// a session is joined and either it is transmitting or a stream is showing.
func (m *Model) videoColumnVisible() bool {
	if m.video == nil {
		return false
	}

	return m.video.tx || len(m.activeVideoTiles()) > 0
}

// sidebarWidth returns the active sidebar column width under the current
// terminal size and video-column state.
func (m *Model) sidebarWidth() int {
	if m.videoColumnVisible() {
		if m.compactMode {
			return 22
		}

		return 30
	}

	if m.compactMode {
		return 16
	}

	return 22
}

// fitWidths derives the derived layout flags and panel widths from the
// terminal size and the video-column state.
func fitWidths(m *Model) {
	m.compactMode = m.width < 100
	m.showSidebar = m.width >= 70

	sw := 0
	if m.showSidebar {
		sw = m.sidebarWidth()
	}

	m.viewport.Width = max(m.width-sw-10, 20)

	m.input.SetWidth(max(m.width-14, 20))
}

// refitLayout recomputes widths and repaints after a layout-affecting
// change such as the video column appearing or disappearing.
func refitLayout(m *Model) {
	fitWidths(m)
	resizeViewport(m)
	rerenderAll(m)
}

func relativeTime(unix int64) string {
	d := time.Since(time.Unix(unix, 0))

	if d <= 0 {
		return "now"
	}

	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}

	return fmt.Sprintf("%dh", int(d.Hours()))
}

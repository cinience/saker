package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// MessageViewport wraps bubbles/v2/viewport to manage scrollable message history.
// It replaces the old tea.Println flush-to-scrollback pattern with an in-view
// scrollable region that works in alt-screen mode.
type MessageViewport struct {
	vp     viewport.Model
	lines  []string
	width  int
	height int
	sticky bool // auto-scroll to bottom on new content

	// "new messages" tracking
	newMsgCount    int
	scrollAwayLine int // line count when user first scrolled away

	// sticky prompt
	stickyPrompt string
}

// NewMessageViewport creates a viewport ready for use.
func NewMessageViewport(width, height int) *MessageViewport {
	vp := viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
	vp.MouseWheelDelta = 3
	return &MessageViewport{
		vp:     vp,
		width:  width,
		height: height,
		sticky: true,
	}
}

// SetSize updates the viewport dimensions.
func (m *MessageViewport) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.vp.SetWidth(width)
	m.vp.SetHeight(height)
}

// AppendRendered adds pre-rendered content to the viewport.
// If sticky (user hasn't scrolled up), auto-scrolls to bottom.
func (m *MessageViewport) AppendRendered(s string) {
	if s == "" {
		return
	}
	newLines := strings.Split(s, "\n")
	m.lines = append(m.lines, newLines...)
	m.vp.SetContent(strings.Join(m.lines, "\n"))
	if m.sticky {
		m.vp.GotoBottom()
	} else {
		m.newMsgCount++
	}
}

// SetContent replaces all viewport content (used for live area re-render).
func (m *MessageViewport) SetContent(s string) {
	m.lines = strings.Split(s, "\n")
	m.vp.SetContent(s)
	if m.sticky {
		m.vp.GotoBottom()
	}
}

// ScrollUp scrolls up by n lines.
func (m *MessageViewport) ScrollUp(n int) {
	m.vp.ScrollUp(n)
	m.sticky = false
	if m.scrollAwayLine == 0 {
		m.scrollAwayLine = len(m.lines)
	}
}

// ScrollDown scrolls down by n lines.
func (m *MessageViewport) ScrollDown(n int) {
	m.vp.ScrollDown(n)
	m.checkSticky()
}

// ScrollHalfPageUp scrolls up by half the viewport height.
func (m *MessageViewport) ScrollHalfPageUp() {
	m.ScrollUp(m.height / 2)
}

// ScrollHalfPageDown scrolls down by half the viewport height.
func (m *MessageViewport) ScrollHalfPageDown() {
	m.ScrollDown(m.height / 2)
}

// GotoTop scrolls to the very top.
func (m *MessageViewport) GotoTop() {
	m.vp.GotoTop()
	m.sticky = false
	if m.scrollAwayLine == 0 {
		m.scrollAwayLine = len(m.lines)
	}
}

// GotoBottom scrolls to the very bottom and re-enables sticky.
func (m *MessageViewport) GotoBottom() {
	m.vp.GotoBottom()
	m.sticky = true
	m.newMsgCount = 0
	m.scrollAwayLine = 0
}

// ScrollToLine scrolls so that the given line index is visible (centered if possible).
func (m *MessageViewport) ScrollToLine(lineIdx int) {
	target := lineIdx - m.height/2
	if target < 0 {
		target = 0
	}
	m.vp.SetYOffset(target)
	m.sticky = false
	if m.scrollAwayLine == 0 {
		m.scrollAwayLine = len(m.lines)
	}
}

// IsAtBottom returns whether the viewport is scrolled to the bottom.
func (m *MessageViewport) IsAtBottom() bool {
	return m.sticky
}

// NewMessageCount returns the number of new messages since the user scrolled away.
func (m *MessageViewport) NewMessageCount() int {
	return m.newMsgCount
}

// SetStickyPrompt sets the user prompt text to show when scrolled up.
func (m *MessageViewport) SetStickyPrompt(s string) {
	m.stickyPrompt = s
}

// Update forwards tea.Msg to the inner viewport for mouse handling.
func (m *MessageViewport) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	m.checkSticky()
	return cmd
}

// View returns the viewport's rendered visible content, including overlays.
func (m *MessageViewport) View() string {
	view := m.vp.View()

	// Overlay: sticky prompt header (when scrolled up)
	if !m.sticky && m.stickyPrompt != "" {
		lines := strings.Split(view, "\n")
		if len(lines) > 0 {
			prompt := m.stickyPrompt
			if lipgloss.Width(prompt) > m.width-4 {
				prompt = prompt[:m.width-7] + "..."
			}
			header := lipgloss.NewStyle().
				Foreground(lipgloss.Color("#999999")).
				Italic(true).
				Render("> " + prompt)
			lines[0] = header
			view = strings.Join(lines, "\n")
		}
	}

	// Overlay: new messages pill (when scrolled up with new content)
	if !m.sticky && m.newMsgCount > 0 {
		lines := strings.Split(view, "\n")
		if len(lines) > 0 {
			pill := newMessagesPill(m.newMsgCount, m.width)
			lines[len(lines)-1] = pill
			view = strings.Join(lines, "\n")
		}
	}

	return view
}

// checkSticky determines if the viewport is at the bottom after a scroll.
func (m *MessageViewport) checkSticky() {
	if m.vp.AtBottom() {
		m.sticky = true
		m.newMsgCount = 0
		m.scrollAwayLine = 0
	}
}

// newMessagesPill renders a centered "N new messages" indicator.
func newMessagesPill(count, width int) string {
	var text string
	if count == 1 {
		text = "↓ 1 new message"
	} else {
		text = fmt.Sprintf("↓ %d new messages", count)
	}
	pill := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#000000")).
		Background(lipgloss.Color("#FFC107")).
		Bold(true).
		Padding(0, 1).
		Render(text)

	pillW := lipgloss.Width(pill)
	pad := (width - pillW) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + pill
}

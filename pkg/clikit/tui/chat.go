package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// maxContentWidth caps text width for readability.
const maxContentWidth = 120

// responsePrefix is the exact Claude Code MessageResponse prefix: "  ⎿  "
const responsePrefix = "  " + IconResponse + "  "

// MsgRole identifies who sent a chat message.
type MsgRole int

const (
	RoleUser MsgRole = iota
	RoleAssistant
	RoleTool
	RoleError
	RoleSystem
	RoleImage
	RoleBtw // /btw side question header
	RoleIM  // /im side question header
)

// ChatMsg represents a single message in the chat history.
type ChatMsg struct {
	Role    MsgRole
	Content string
	// Tool-specific fields.
	ToolName     string
	ToolParams   string // brief parameter summary
	ToolStatus   string // "pending", "success", "error"
	ToolOutput   string // result summary (e.g., "Read 42 lines", output preview)
	ToolDetail   string // multi-line output preview (first few lines of raw output)

	// Image-specific fields (RoleImage).
	ImagePath string
}

// Chat manages the chat message area.
// Completed messages are flushed to terminal scrollback via tea.Println;
// only unflushed messages and the streaming buffer appear in the live View().
type Chat struct {
	styles   Styles
	messages []ChatMsg
	flushed  int // index of first unflushed message
	width    int
	height   int // terminal height for dynamic flush threshold

	// streaming accumulator for the current assistant response
	streaming       bool
	streamingBuffer strings.Builder
	streamRenderer  *StreamingRenderer
}

// NewChat creates a Chat component.
func NewChat(s Styles) *Chat {
	return &Chat{styles: s, streamRenderer: NewStreamingRenderer()}
}

// SetWidth updates the chat rendering width.
func (c *Chat) SetWidth(w int) {
	c.width = w
}

// SetHeight updates the terminal height for dynamic flush threshold.
func (c *Chat) SetHeight(h int) {
	c.height = h
}

// FlushThreshold returns the max streaming lines before flushing.
func (c *Chat) FlushThreshold() int {
	h := c.height - 6 // reserve space for input, status, spinner
	if h < 8 {
		h = 8
	}
	return h
}

// Clear resets the chat state (e.g., on /new).
func (c *Chat) Clear() {
	c.messages = nil
	c.flushed = 0
	c.streaming = false
	c.streamingBuffer.Reset()
}

// AddUserMessage adds a user message to the chat.
func (c *Chat) AddUserMessage(text string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleUser, Content: text})
}

// StartStreaming begins accumulating assistant text.
func (c *Chat) StartStreaming() {
	c.streaming = true
	c.streamingBuffer.Reset()
	c.streamRenderer.Reset()
}

// AppendStreamText adds a text delta to the current streaming response.
func (c *Chat) AppendStreamText(text string) {
	c.streamingBuffer.WriteString(text)
}

// StreamingLineCount returns the number of newlines in the streaming buffer.
func (c *Chat) StreamingLineCount() int {
	return strings.Count(c.streamingBuffer.String(), "\n")
}

// FinishStreaming finalises the current streaming response as a message.
// Trivial content (whitespace, lone punctuation) between tool calls is discarded
// to avoid rendering empty "● ." blocks.
func (c *Chat) FinishStreaming() {
	if c.streamingBuffer.Len() > 0 {
		content := c.streamingBuffer.String()
		trimmed := strings.TrimSpace(content)
		// Only create a message if there's meaningful text.
		// Skip trivial inter-tool deltas like ".", "..", whitespace-only, etc.
		if len(trimmed) > 2 || (trimmed != "" && !isTrivialDelta(trimmed)) {
			c.messages = append(c.messages, ChatMsg{
				Role:    RoleAssistant,
				Content: content,
			})
		}
	}
	c.streaming = false
	c.streamingBuffer.Reset()
}

// isTrivialDelta returns true for content that is just punctuation dots or
// whitespace — these are artifacts from model deltas between tool calls.
func isTrivialDelta(s string) bool {
	for _, r := range s {
		if r != '.' && r != ' ' && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}

// AddToolCall adds a tool call message.
func (c *Chat) AddToolCall(name, status string) {
	c.messages = append(c.messages, ChatMsg{
		Role:       RoleTool,
		ToolName:   name,
		ToolStatus: status,
	})
}

// AddToolCallWithParams adds a tool call with parameter summary.
func (c *Chat) AddToolCallWithParams(name, params, status string) {
	c.messages = append(c.messages, ChatMsg{
		Role:       RoleTool,
		ToolName:   name,
		ToolParams: params,
		ToolStatus: status,
	})
}

// UpdateLastToolStatus updates the status of the most recent tool message.
func (c *Chat) UpdateLastToolStatus(status string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == RoleTool {
			c.messages[i].ToolStatus = status
			break
		}
	}
}

// UpdateLastToolOutput sets the output summary of the most recent tool message.
func (c *Chat) UpdateLastToolOutput(output string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == RoleTool {
			c.messages[i].ToolOutput = output
			break
		}
	}
}

// UpdateLastToolDetail sets the detail preview of the most recent tool message.
func (c *Chat) UpdateLastToolDetail(detail string) {
	for i := len(c.messages) - 1; i >= 0; i-- {
		if c.messages[i].Role == RoleTool {
			c.messages[i].ToolDetail = detail
			break
		}
	}
}

// AddBtwQuestion adds a /btw side question header (Claude Code style: yellow "/btw" + dim question).
func (c *Chat) AddBtwQuestion(question string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleBtw, Content: question})
}

// AddIMQuestion adds an /im side question header (cyan "/im" + dim instruction).
func (c *Chat) AddIMQuestion(instruction string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleIM, Content: instruction})
}

// AddError adds an error message.
func (c *Chat) AddError(text string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleError, Content: text})
}

// AddSystem adds a system info message (non-error).
func (c *Chat) AddSystem(text string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleSystem, Content: text})
}

// AddImage adds an inline image message.
func (c *Chat) AddImage(path string) {
	c.messages = append(c.messages, ChatMsg{Role: RoleImage, ImagePath: path})
}

// FlushMessages renders all unflushed completed messages and marks them flushed.
// The returned string is meant to be printed via tea.Println so it enters the
// terminal scrollback and is no longer part of the live rendering area.
func (c *Chat) FlushMessages() (string, bool) {
	if c.flushed >= len(c.messages) {
		return "", false
	}
	var b strings.Builder
	c.renderMessages(&b, c.flushed, len(c.messages))
	c.flushed = len(c.messages)
	return b.String(), true
}

// View returns the rendered content for the live area.
// This includes unflushed messages (e.g., tool calls during streaming)
// and the current streaming buffer.
func (c *Chat) View() string {
	var b strings.Builder

	// Render unflushed messages (visible during active streaming).
	c.renderMessages(&b, c.flushed, len(c.messages))

	// Streaming buffer — ● dot on first line, space indent on continuation,
	// cursor appended to the trailing line (no extra empty cursor row).
	if c.streaming && c.streamingBuffer.Len() > 0 {
		cw := c.contentWidth()
		text := strings.TrimRight(c.streamingBuffer.String(), "\n")
		rendered := c.streamRenderer.Render(text, cw)
		cursor := lipgloss.NewStyle().Foreground(c.styles.Theme.Primary).Render(IconCursor)
		lines := strings.Split(rendered, "\n")
		lastIdx := len(lines) - 1
		for i, line := range lines {
			suffix := ""
			if i == lastIdx {
				suffix = " " + cursor
			}
			if i == 0 {
				dot := c.styles.AssistantDot.Render(IconCircle)
				fmt.Fprintf(&b, "%s %s%s\n", dot, line, suffix)
			} else {
				fmt.Fprintf(&b, " %s%s\n", line, suffix)
			}
		}
	}

	return b.String()
}

// renderMessages renders messages[from:to] into the builder.
func (c *Chat) renderMessages(b *strings.Builder, from, to int) {
	cw := c.contentWidth()

	// Pre-compute collapsed groups for this range.
	groups := CollapseMessages(c.messages[from:to])
	collapsed := make(map[int]bool) // indices within [from:to] that are part of a collapsed group
	groupStart := make(map[int]*CollapsedGroup)
	for gi := range groups {
		g := &groups[gi]
		groupStart[g.StartIdx] = g
		for idx := g.StartIdx; idx <= g.EndIdx; idx++ {
			collapsed[idx] = true
		}
	}

	for i := from; i < to; i++ {
		ri := i - from // relative index

		// If this is the start of a collapsed group, render the summary.
		if g, ok := groupStart[ri]; ok {
			b.WriteString(RenderCollapsedGroup(*g, c.styles))
			b.WriteString("\n")
			i = from + g.EndIdx // skip to end of group
			continue
		}
		// Skip interior items of a collapsed group.
		if collapsed[ri] {
			continue
		}

		msg := c.messages[i]
		switch msg.Role {
		case RoleUser:
			c.renderUser(b, msg, cw)
		case RoleAssistant:
			c.renderAssistant(b, msg, cw)
		case RoleTool:
			c.renderTool(b, msg)
		case RoleError:
			c.renderError(b, msg, cw)
		case RoleSystem:
			c.renderSystem(b, msg, cw)
		case RoleImage:
			c.renderImage(b, msg, cw)
		case RoleBtw:
			c.renderBtw(b, msg, cw)
		case RoleIM:
			c.renderIM(b, msg, cw)
		}

		// Only add a blank separator line before a new user turn.
		if i+1 < len(c.messages) {
			nextRole := c.messages[i+1].Role
			if nextRole == RoleUser || nextRole == RoleBtw || nextRole == RoleIM {
				b.WriteString("\n")
			}
		}
	}
}

// contentWidth returns the capped width for message content.
func (c *Chat) contentWidth() int {
	w := c.width - 2 // room for "● " prefix
	if w < 20 {
		w = 20
	}
	if w > maxContentWidth {
		w = maxContentWidth
	}
	return w
}

// renderUser renders a user message with background color (Claude Code style).
// The trailing newline is owned by renderMessages so adjacent message spacing
// stays consistent.
func (c *Chat) renderUser(b *strings.Builder, msg ChatMsg, width int) {
	text := c.styles.UserMsgBlock.Width(c.width).Render(
		fmt.Sprintf(" %s", msg.Content),
	)
	b.WriteString(text)
	b.WriteString("\n")
}

// renderAssistant renders an assistant message with ● dot on first line.
// Continuation lines use space indentation only (no ⎿ symbol).
// The ⎿ prefix is reserved for tool calls, keeping text clean and readable.
//
// Leading blank lines (often inserted by glamour's `document.block_prefix`)
// are dropped, and runs of consecutive blank lines are folded down to one to
// keep the chat area dense.
func (c *Chat) renderAssistant(b *strings.Builder, msg ChatMsg, width int) {
	rendered := renderMarkdown(msg.Content, width)
	lines := strings.Split(rendered, "\n")

	firstWritten := false
	prevBlank := false
	for _, line := range lines {
		isBlank := strings.TrimSpace(line) == ""
		if isBlank && (!firstWritten || prevBlank) {
			continue
		}
		prevBlank = isBlank

		// Ensure lines without explicit ANSI color get bright foreground.
		displayLine := line
		if !strings.Contains(line, "\033[") {
			displayLine = c.styles.AssistantText.Render(line)
		}

		if !firstWritten {
			dot := c.styles.AssistantDot.Render(IconCircle)
			fmt.Fprintf(b, "%s %s\n", dot, displayLine)
			firstWritten = true
			continue
		}
		fmt.Fprintf(b, " %s\n", displayLine)
	}
}

// renderTool renders a tool call compactly:
//
//	Pending:   ⎿  ● Read (file.go) …
//	Complete:  ⎿  ✓ Read (file.go) — 42 lines
//	Error:     ⎿  ✗ Read (file.go) — error message
func (c *Chat) renderTool(b *strings.Builder, msg ChatMsg) {
	dot := c.styles.AssistantDot.Render(IconCircle)

	// Build: icon + name + (params)
	parts := []string{
		c.toolIcon(msg.ToolStatus),
		c.styles.ToolName.Render(msg.ToolName),
	}
	if msg.ToolParams != "" {
		parts = append(parts, c.styles.ToolParam.Render("("+msg.ToolParams+")"))
	}

	// Append status/result inline for compact display
	switch msg.ToolStatus {
	case "pending":
		parts = append(parts, c.styles.ToolParam.Render("…"))
	case "success", "error":
		if msg.ToolOutput != "" {
			parts = append(parts, c.styles.ToolParam.Render("— "+msg.ToolOutput))
		}
	}

	fmt.Fprintf(b, "%s %s\n", dot, strings.Join(parts, " "))

	// Show detail preview with ⎿ continuation prefix
	if msg.ToolDetail != "" && msg.ToolStatus != "pending" {
		prefix := c.styles.ResponsePrefix.Render(responsePrefix)
		detailLines := strings.Split(msg.ToolDetail, "\n")
		detailStyle := lipgloss.NewStyle().Foreground(c.styles.Theme.FgDim)
		for _, line := range detailLines {
			fmt.Fprintf(b, "%s%s\n", prefix, detailStyle.Render(line))
		}
	}
}

// renderError renders an error message left-aligned.
func (c *Chat) renderError(b *strings.Builder, msg ChatMsg, width int) {
	text := c.styles.ErrorText.Width(width).Render(msg.Content)
	fmt.Fprintf(b, "%s\n", text)
}

// renderSystem renders a system info message left-aligned.
func (c *Chat) renderSystem(b *strings.Builder, msg ChatMsg, width int) {
	text := c.styles.SystemText.Width(width).Render(msg.Content)
	fmt.Fprintf(b, "%s\n", text)
}

// renderImage renders an inline image left-aligned.
func (c *Chat) renderImage(b *strings.Builder, msg ChatMsg, width int) {
	pathStyle := lipgloss.NewStyle().Foreground(c.styles.Theme.Secondary)
	labelStyle := lipgloss.NewStyle().Foreground(c.styles.Theme.FgDim)
	fmt.Fprintf(b, "%s %s\n", labelStyle.Render("Image:"), pathStyle.Render(msg.ImagePath))

	proto := DetectImageProtocol()
	if proto != ProtocolNone {
		img := RenderImage(msg.ImagePath, width/2)
		if img != "" && !strings.HasPrefix(img, "[Image:") {
			fmt.Fprintf(b, "%s\n", img)
		}
	}
}

// renderBtw renders a /btw side question header left-aligned.
func (c *Chat) renderBtw(b *strings.Builder, msg ChatMsg, width int) {
	btwLabel := lipgloss.NewStyle().Foreground(c.styles.Theme.Warning).Bold(true).Render("/btw ")
	question := lipgloss.NewStyle().Foreground(c.styles.Theme.FgDim).Width(width).Render(msg.Content)
	fmt.Fprintf(b, "%s%s\n", btwLabel, question)
}

// renderIM renders an /im side question header left-aligned.
func (c *Chat) renderIM(b *strings.Builder, msg ChatMsg, width int) {
	imLabel := lipgloss.NewStyle().Foreground(c.styles.Theme.Accent).Bold(true).Render("/im ")
	question := lipgloss.NewStyle().Foreground(c.styles.Theme.FgDim).Width(width).Render(msg.Content)
	fmt.Fprintf(b, "%s%s\n", imLabel, question)
}

func (c *Chat) toolIcon(status string) string {
	switch status {
	case "success":
		return c.styles.ToolSuccess.Render(IconCheck)
	case "error":
		return c.styles.ToolError.Render(IconError)
	case "queued":
		return c.styles.ToolPending.Render(IconDot)
	default:
		return c.styles.ToolPending.Render(IconPending)
	}
}

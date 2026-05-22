package tui

import (
	"strings"
	"testing"
)

func TestStreamingViewHeightCap(t *testing.T) {
	theme := DefaultTheme()
	SetMarkdownTheme(theme)
	styles := NewStyles(theme)
	chat := NewChat(styles)
	chat.SetWidth(80)
	chat.SetHeight(30) // simulate 30-line terminal

	chat.StartStreaming()
	// Generate streaming content that exceeds terminal height.
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("This is a line of streaming content for testing purposes.\n")
	}
	chat.AppendStreamText(sb.String())

	view := chat.View()
	viewLines := strings.Split(strings.TrimRight(view, "\n"), "\n")

	// maxLines = height - 5 = 25
	maxLines := 30 - 5
	if len(viewLines) > maxLines {
		t.Errorf("View() output has %d lines, should be capped at %d (height=%d)",
			len(viewLines), maxLines, 30)
	}
}

func TestStreamingViewKeepsTail(t *testing.T) {
	theme := DefaultTheme()
	SetMarkdownTheme(theme)
	styles := NewStyles(theme)
	chat := NewChat(styles)
	chat.SetWidth(80)
	chat.SetHeight(20)

	chat.StartStreaming()
	var sb strings.Builder
	for i := 1; i <= 40; i++ {
		sb.WriteString(strings.Repeat("x", 10))
		sb.WriteString("\n")
	}
	chat.AppendStreamText(sb.String())

	view := chat.View()
	plain := stripANSI(view)

	// The tail (last lines) should be visible, not the head.
	// Last line of streaming content is "xxxxxxxxxx" — should be present.
	if !strings.Contains(plain, "xxxxxxxxxx") {
		t.Errorf("View() should show tail of streaming content, got %q", plain)
	}
}

func TestStreamingViewNoDotWhenTruncated(t *testing.T) {
	theme := DefaultTheme()
	SetMarkdownTheme(theme)
	styles := NewStyles(theme)
	chat := NewChat(styles)
	chat.SetWidth(80)
	chat.SetHeight(20) // maxLines = 15

	chat.AddUserMessage("hello")
	chat.FlushMessages() // flush user msg

	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("streaming line\n")
	}
	chat.AppendStreamText(sb.String())

	view := chat.View()
	plain := stripANSI(view)

	// When truncated, the first line of the turn (●) is scrolled away.
	// No ● should appear in the visible output.
	if strings.Contains(plain, IconCircle) {
		t.Errorf("truncated view should not show ● dot, got %q", plain)
	}
}

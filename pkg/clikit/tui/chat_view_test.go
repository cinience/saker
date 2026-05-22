package tui

import (
	"fmt"
	"strings"
	"testing"
)

func newTestChat(width, height int) *Chat {
	theme := DefaultTheme()
	SetMarkdownTheme(theme)
	styles := NewStyles(theme)
	chat := NewChat(styles)
	chat.SetWidth(width)
	chat.SetHeight(height)
	return chat
}

func viewLineCount(chat *Chat) int {
	view := chat.View()
	if view == "" {
		return 0
	}
	return len(strings.Split(strings.TrimRight(view, "\n"), "\n"))
}

// --- Streaming height cap ---

func TestStreamingViewHeightCap(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 50; i++ {
		sb.WriteString("line of streaming content\n")
	}
	chat.AppendStreamText(sb.String())

	lines := viewLineCount(chat)
	maxLines := 30 - 5
	if lines > maxLines {
		t.Errorf("View() has %d lines, want <= %d", lines, maxLines)
	}
}

func TestStreamingViewFitsNoCap(t *testing.T) {
	chat := newTestChat(80, 40)
	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString("short content\n")
	}
	chat.AppendStreamText(sb.String())

	lines := viewLineCount(chat)
	if lines != 10 {
		t.Errorf("View() has %d lines, want 10 (no truncation needed)", lines)
	}
}

func TestStreamingViewMinimumCap(t *testing.T) {
	chat := newTestChat(80, 10) // height-5 = 5, but minimum is 8
	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("line\n")
	}
	chat.AppendStreamText(sb.String())

	lines := viewLineCount(chat)
	if lines > 8 {
		t.Errorf("View() has %d lines, want <= 8 (minimum cap)", lines)
	}
}

func TestStreamingViewZeroHeight(t *testing.T) {
	chat := newTestChat(80, 0) // no WindowSizeMsg yet
	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 20; i++ {
		sb.WriteString("line\n")
	}
	chat.AppendStreamText(sb.String())

	lines := viewLineCount(chat)
	if lines > 8 {
		t.Errorf("View() has %d lines, want <= 8 (zero height → minimum cap)", lines)
	}
}

// --- Tail preservation ---

func TestStreamingViewKeepsTail(t *testing.T) {
	chat := newTestChat(80, 20)
	chat.StartStreaming()
	for i := 1; i <= 40; i++ {
		chat.AppendStreamText(fmt.Sprintf("LINE_%04d\n", i))
	}

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "LINE_0040") {
		t.Error("tail line LINE_0040 should be visible")
	}
	if !strings.Contains(plain, "LINE_0039") {
		t.Error("near-tail line LINE_0039 should be visible")
	}
	if strings.Contains(plain, "LINE_0001") {
		t.Error("head line LINE_0001 should be truncated away")
	}
}

// --- Cursor position ---

func TestStreamingViewCursorOnLastLine(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	chat.AppendStreamText("first line\nsecond line\nthird line\n")

	view := chat.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, IconCursor) {
		t.Errorf("cursor should be on the last line, got %q", lastLine)
	}
	for i := 0; i < len(lines)-1; i++ {
		if strings.Contains(lines[i], IconCursor) {
			t.Errorf("cursor should NOT be on line %d: %q", i, lines[i])
		}
	}
}

func TestStreamingViewCursorOnLastLineWhenTruncated(t *testing.T) {
	chat := newTestChat(80, 15)
	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("content line\n")
	}
	chat.AppendStreamText(sb.String())

	view := chat.View()
	lines := strings.Split(strings.TrimRight(view, "\n"), "\n")
	lastLine := lines[len(lines)-1]
	if !strings.Contains(lastLine, IconCursor) {
		t.Errorf("cursor should be on the last line even when truncated")
	}
}

// --- ● dot behavior ---

func TestStreamingViewDotFirstInTurn(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.FlushMessages()

	chat.StartStreaming()
	chat.AppendStreamText("response text\n")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, IconCircle) {
		t.Error("first assistant text in turn should show ● dot")
	}
}

func TestStreamingViewNoDotWhenTruncated(t *testing.T) {
	chat := newTestChat(80, 20)
	chat.AddUserMessage("hello")
	chat.FlushMessages()

	chat.StartStreaming()
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("streaming line\n")
	}
	chat.AppendStreamText(sb.String())

	plain := stripANSI(chat.View())
	if strings.Contains(plain, IconCircle) {
		t.Error("truncated view should not show ● dot")
	}
}

func TestStreamingViewNoDotOnContinuation(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.FlushMessages()

	// First assistant text + tool call → continuation
	chat.StartStreaming()
	chat.AppendStreamText("first part")
	chat.FinishStreaming()
	chat.AddToolCallWithParams("Read", "file.go", "success")
	chat.FlushMessages() // flush prior messages so only streaming is in View()

	// New streaming is a continuation (not first in turn)
	chat.StartStreaming()
	chat.AppendStreamText("continuation text\n")

	plain := stripANSI(chat.View())
	if strings.Contains(plain, IconCircle) {
		t.Error("continuation streaming should not show ● dot")
	}
}

func TestAssistantMessageDotFirstInTurn(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.FlushMessages()

	chat.StartStreaming()
	chat.AppendStreamText("response text")
	chat.FinishStreaming()

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, IconCircle) {
		t.Error("first assistant message in turn should show ● dot")
	}
}

func TestAssistantMessageNoDotOnContinuation(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.FlushMessages()

	// First part
	chat.StartStreaming()
	chat.AppendStreamText("first part")
	chat.FinishStreaming()

	// Tool call
	chat.AddToolCallWithParams("Read", "file.go", "success")

	// Second assistant text → continuation, no dot
	chat.StartStreaming()
	chat.AppendStreamText("second part")
	chat.FinishStreaming()

	view := chat.View()
	plain := stripANSI(view)
	lines := strings.Split(strings.TrimRight(plain, "\n"), "\n")

	// First unflushed assistant message gets ● (it's the first assistant in turn)
	// The second one should NOT get ● (continuation after tool call)
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, IconCircle) {
			dotCount++
		}
	}
	if dotCount != 1 {
		t.Errorf("expected exactly 1 ● dot, got %d, view:\n%s", dotCount, plain)
	}
}

// --- Flush lifecycle ---

func TestFlushRemovesFromView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.StartStreaming()
	chat.AppendStreamText("response")
	chat.FinishStreaming()

	// Before flush: both user msg and assistant msg in view
	view1 := stripANSI(chat.View())
	if !strings.Contains(view1, "response") {
		t.Error("unflushed assistant message should be in View()")
	}

	// Flush
	flushed, ok := chat.FlushMessages()
	if !ok {
		t.Error("FlushMessages should return ok=true")
	}
	if !strings.Contains(stripANSI(flushed), "response") {
		t.Error("flushed content should contain assistant message")
	}

	// After flush: view is empty
	view2 := chat.View()
	if strings.TrimSpace(view2) != "" {
		t.Errorf("View() should be empty after flush, got %q", view2)
	}
}

func TestDoubleFlushReturnsNothing(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")

	_, ok1 := chat.FlushMessages()
	if !ok1 {
		t.Error("first FlushMessages should return ok=true")
	}

	_, ok2 := chat.FlushMessages()
	if ok2 {
		t.Error("second FlushMessages should return ok=false (nothing to flush)")
	}
}

func TestFlushPreservesStreamingBuffer(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.StartStreaming()
	chat.AppendStreamText("still streaming")

	// Flush should flush user message but streaming buffer stays
	chat.FlushMessages()

	view := chat.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "still streaming") {
		t.Error("streaming buffer should survive flush")
	}
}

// --- FinishStreaming ---

func TestFinishStreamingConvertsToMessage(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	chat.AppendStreamText("completed response")
	chat.FinishStreaming()

	view := stripANSI(chat.View())
	if !strings.Contains(view, "completed response") {
		t.Error("finished streaming should create an unflushed assistant message")
	}

	// Streaming is off
	if chat.streaming {
		t.Error("streaming should be false after FinishStreaming")
	}
}

func TestFinishStreamingDiscardsTrivialDelta(t *testing.T) {
	trivials := []string{".", "..", "  ", "\n", " . ", "\t"}
	for _, input := range trivials {
		chat := newTestChat(80, 30)
		chat.StartStreaming()
		chat.AppendStreamText(input)
		chat.FinishStreaming()

		view := strings.TrimSpace(chat.View())
		if view != "" {
			t.Errorf("trivial delta %q should be discarded, got View()=%q", input, view)
		}
	}
}

func TestFinishStreamingKeepsMeaningfulContent(t *testing.T) {
	meaningful := []string{"OK", "hi", "yes", "a b c"}
	for _, input := range meaningful {
		chat := newTestChat(80, 30)
		chat.StartStreaming()
		chat.AppendStreamText(input)
		chat.FinishStreaming()

		view := strings.TrimSpace(stripANSI(chat.View()))
		if !strings.Contains(view, strings.TrimSpace(input)) {
			t.Errorf("meaningful content %q should be kept, got View()=%q", input, view)
		}
	}
}

// --- Tool messages ---

func TestToolMessageRendersInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "file.go", "success")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "Read") {
		t.Error("unflushed tool message should appear in View()")
	}
	if !strings.Contains(plain, "file.go") {
		t.Error("tool params should appear in View()")
	}
}

func TestToolMessageFlushedRemoved(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "file.go", "success")
	chat.FlushMessages()

	view := strings.TrimSpace(chat.View())
	if view != "" {
		t.Errorf("flushed tool message should not appear in View(), got %q", view)
	}
}

func TestToolStatusUpdate(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Bash", "ls", "pending")
	chat.UpdateLastToolStatus("success")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, IconCheck) {
		t.Error("success status should show ✓ icon")
	}
}

func TestToolOutputUpdate(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "file.go", "success")
	chat.UpdateLastToolOutput("42 lines")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "42 lines") {
		t.Error("tool output should appear in View()")
	}
}

func TestToolDetailRendered(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Bash", "ls", "success")
	chat.UpdateLastToolDetail("file1.go\nfile2.go")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "file1.go") || !strings.Contains(plain, "file2.go") {
		t.Error("tool detail lines should appear in View()")
	}
}

func TestToolDetailHiddenWhenPending(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Bash", "ls", "pending")
	chat.UpdateLastToolDetail("should not show")

	plain := stripANSI(chat.View())
	if strings.Contains(plain, "should not show") {
		t.Error("tool detail should be hidden while status is pending")
	}
}

// --- Combined unflushed + streaming ---

func TestUnflushedPlusStreamingInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "file.go", "success")
	chat.StartStreaming()
	chat.AppendStreamText("streaming response\n")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "Read") {
		t.Error("unflushed tool message should appear with streaming content")
	}
	if !strings.Contains(plain, "streaming response") {
		t.Error("streaming content should appear with unflushed messages")
	}
}

// --- Empty states ---

func TestEmptyView(t *testing.T) {
	chat := newTestChat(80, 30)
	view := chat.View()
	if view != "" {
		t.Errorf("empty chat should produce empty View(), got %q", view)
	}
}

func TestStreamingStartedNoText(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	view := chat.View()
	if view != "" {
		t.Errorf("streaming with no text should produce empty View(), got %q", view)
	}
}

// --- StreamingLineCount ---

func TestStreamingLineCount(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	chat.AppendStreamText("one\ntwo\nthree\n")

	if got := chat.StreamingLineCount(); got != 3 {
		t.Errorf("StreamingLineCount() = %d, want 3", got)
	}
}

func TestStreamingLineCountEmpty(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()

	if got := chat.StreamingLineCount(); got != 0 {
		t.Errorf("StreamingLineCount() = %d, want 0", got)
	}
}

// --- Clear ---

func TestClearResetsEverything(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.StartStreaming()
	chat.AppendStreamText("response")
	chat.FinishStreaming()

	chat.Clear()

	view := chat.View()
	if view != "" {
		t.Errorf("View() should be empty after Clear(), got %q", view)
	}

	_, ok := chat.FlushMessages()
	if ok {
		t.Error("FlushMessages should return ok=false after Clear()")
	}
}

// --- Error and system messages ---

func TestErrorMessageInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddError("something went wrong")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "something went wrong") {
		t.Error("error message should appear in View()")
	}
}

func TestSystemMessageInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddSystem("info message")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "info message") {
		t.Error("system message should appear in View()")
	}
}

// --- Collapsed groups ---

func TestCollapsedGroupsInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "a.go", "success")
	chat.AddToolCallWithParams("Read", "b.go", "success")
	chat.AddToolCallWithParams("Read", "c.go", "success")

	plain := stripANSI(chat.View())
	// Collapsed group should show count
	if !strings.Contains(plain, "3") {
		t.Error("collapsed group should show count of 3")
	}
	if !strings.Contains(plain, "files read") {
		t.Error("collapsed group should show 'files read' verb")
	}
}

func TestNoCollapseForSingleTool(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "a.go", "success")

	plain := stripANSI(chat.View())
	if strings.Contains(plain, "files read") {
		t.Error("single tool call should not be collapsed")
	}
	if !strings.Contains(plain, "Read") {
		t.Error("single tool call should show tool name")
	}
}

func TestCollapseBreaksOnDifferentToolType(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddToolCallWithParams("Read", "a.go", "success")
	chat.AddToolCallWithParams("Read", "b.go", "success")
	chat.AddToolCallWithParams("Bash", "ls", "success")
	chat.AddToolCallWithParams("Read", "c.go", "success")

	plain := stripANSI(chat.View())
	// First group: 2 reads; then standalone bash; then standalone read
	if !strings.Contains(plain, "2") {
		t.Error("first group should show count 2")
	}
	if !strings.Contains(plain, "Bash") {
		t.Error("standalone Bash should appear individually")
	}
}

// --- Btw / IM messages ---

func TestBtwMessageInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddBtwQuestion("side question")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "/btw") {
		t.Error("btw message should show /btw label")
	}
	if !strings.Contains(plain, "side question") {
		t.Error("btw message should show question content")
	}
}

func TestIMMessageInView(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddIMQuestion("instruction")

	plain := stripANSI(chat.View())
	if !strings.Contains(plain, "/im") {
		t.Error("im message should show /im label")
	}
	if !strings.Contains(plain, "instruction") {
		t.Error("im message should show instruction content")
	}
}

// --- Separator lines ---

func TestBlankLineBetweenTurns(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.StartStreaming()
	chat.AppendStreamText("assistant reply")
	chat.FinishStreaming()
	chat.AddUserMessage("follow up")

	view := chat.View()
	plain := stripANSI(view)
	// There should be a blank line before the user message
	if !strings.Contains(plain, "\n\n") {
		t.Error("there should be a blank separator between assistant and next user turn")
	}
}

// --- isFirstAssistantInTurn ---

func TestIsFirstAssistantInTurnAtStart(t *testing.T) {
	chat := newTestChat(80, 30)
	// No messages at all → first assistant
	if !chat.isFirstAssistantInTurn(0) {
		t.Error("should be first in turn when no messages exist")
	}
}

func TestIsFirstAssistantAfterUser(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	if !chat.isFirstAssistantInTurn(1) {
		t.Error("should be first in turn right after user message")
	}
}

func TestIsNotFirstAfterAssistant(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.StartStreaming()
	chat.AppendStreamText("first")
	chat.FinishStreaming()
	// Index 2 is after assistant at index 1
	if chat.isFirstAssistantInTurn(2) {
		t.Error("should NOT be first in turn after another assistant message")
	}
}

func TestIsNotFirstAfterToolCall(t *testing.T) {
	chat := newTestChat(80, 30)
	chat.AddUserMessage("hello")
	chat.AddToolCallWithParams("Read", "f.go", "success")
	// Index 2 is after tool at index 1
	if chat.isFirstAssistantInTurn(2) {
		t.Error("should NOT be first in turn after a tool call")
	}
}

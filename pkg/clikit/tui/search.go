package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// SearchMatch represents a single search match location.
type SearchMatch struct {
	LineIdx int
	Offset  int
}

// TranscriptSearch provides keyword search across the viewport content.
type TranscriptSearch struct {
	query   string
	matches []SearchMatch
	current int
	active  bool
	styles  Styles
}

// NewTranscriptSearch creates a new search instance.
func NewTranscriptSearch(styles Styles) *TranscriptSearch {
	return &TranscriptSearch{styles: styles}
}

// Activate enters search mode.
func (ts *TranscriptSearch) Activate() {
	ts.active = true
	ts.query = ""
	ts.matches = nil
	ts.current = 0
}

// Deactivate exits search mode.
func (ts *TranscriptSearch) Deactivate() {
	ts.active = false
	ts.query = ""
	ts.matches = nil
	ts.current = 0
}

// IsActive returns whether search mode is active.
func (ts *TranscriptSearch) IsActive() bool {
	return ts.active
}

// SetQuery updates the search query and recomputes matches.
func (ts *TranscriptSearch) SetQuery(q string, lines []string) {
	ts.query = q
	ts.matches = nil
	ts.current = 0

	if q == "" {
		return
	}

	lower := strings.ToLower(q)
	for i, line := range lines {
		lineLower := strings.ToLower(line)
		offset := 0
		for {
			idx := strings.Index(lineLower[offset:], lower)
			if idx < 0 {
				break
			}
			ts.matches = append(ts.matches, SearchMatch{LineIdx: i, Offset: offset + idx})
			offset += idx + len(lower)
		}
	}
}

// AddChar appends a character to the query.
func (ts *TranscriptSearch) AddChar(ch rune, lines []string) {
	ts.SetQuery(ts.query+string(ch), lines)
}

// DeleteChar removes the last character from the query.
func (ts *TranscriptSearch) DeleteChar(lines []string) {
	if len(ts.query) > 0 {
		runes := []rune(ts.query)
		ts.SetQuery(string(runes[:len(runes)-1]), lines)
	}
}

// NextMatch advances to the next match.
func (ts *TranscriptSearch) NextMatch() {
	if len(ts.matches) == 0 {
		return
	}
	ts.current = (ts.current + 1) % len(ts.matches)
}

// PrevMatch goes to the previous match.
func (ts *TranscriptSearch) PrevMatch() {
	if len(ts.matches) == 0 {
		return
	}
	ts.current = (ts.current - 1 + len(ts.matches)) % len(ts.matches)
}

// CurrentLine returns the line index of the current match, or -1.
func (ts *TranscriptSearch) CurrentLine() int {
	if len(ts.matches) == 0 {
		return -1
	}
	return ts.matches[ts.current].LineIdx
}

// Query returns the current search query.
func (ts *TranscriptSearch) Query() string {
	return ts.query
}

// MatchCount returns total matches found.
func (ts *TranscriptSearch) MatchCount() int {
	return len(ts.matches)
}

// CurrentIndex returns the 1-based index of the current match.
func (ts *TranscriptSearch) CurrentIndex() int {
	if len(ts.matches) == 0 {
		return 0
	}
	return ts.current + 1
}

// StatusView renders the search bar (shown at bottom of viewport).
func (ts *TranscriptSearch) StatusView(width int) string {
	if !ts.active {
		return ""
	}

	queryStyle := lipgloss.NewStyle().Foreground(ts.styles.Theme.Fg)
	countStyle := lipgloss.NewStyle().Foreground(ts.styles.Theme.FgDim)

	left := queryStyle.Render("/" + ts.query)

	var right string
	if len(ts.matches) > 0 {
		right = countStyle.Render(fmt.Sprintf("[%d/%d]", ts.current+1, len(ts.matches)))
	} else if ts.query != "" {
		right = countStyle.Render("[no matches]")
	}

	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	return left + strings.Repeat(" ", gap) + right
}

// HighlightLine applies search highlighting to a single line of text.
func (ts *TranscriptSearch) HighlightLine(line string, lineIdx int) string {
	if !ts.active || ts.query == "" || len(ts.matches) == 0 {
		return line
	}

	lower := strings.ToLower(line)
	queryLower := strings.ToLower(ts.query)
	qLen := len(ts.query)

	currentHighlight := lipgloss.NewStyle().
		Background(lipgloss.Color("#FFC107")).
		Foreground(lipgloss.Color("#000000"))
	otherHighlight := lipgloss.NewStyle().
		Background(lipgloss.Color("#665000")).
		Foreground(lipgloss.Color("#FFFFFF"))

	var result strings.Builder
	pos := 0
	for {
		idx := strings.Index(lower[pos:], queryLower)
		if idx < 0 {
			result.WriteString(line[pos:])
			break
		}

		result.WriteString(line[pos : pos+idx])

		matchText := line[pos+idx : pos+idx+qLen]
		isCurrent := false
		for _, m := range ts.matches {
			if m.LineIdx == lineIdx && m.Offset == pos+idx && ts.matches[ts.current].LineIdx == lineIdx && ts.matches[ts.current].Offset == pos+idx {
				isCurrent = true
				break
			}
		}

		if isCurrent {
			result.WriteString(currentHighlight.Render(matchText))
		} else {
			result.WriteString(otherHighlight.Render(matchText))
		}
		pos += idx + qLen
	}

	return result.String()
}

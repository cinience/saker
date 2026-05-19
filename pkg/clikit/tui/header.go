package tui

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// mascotRows is a stylised saker falcon (猎隼) emblem.
var mascotRows = [3]string{
	`   ▗▄███▄▖   `,
	`   ▐◈ ▼ ◈▌   `,
	`    ▀▄█▄▀    `,
}

// Header renders the top section of the TUI with a Claude Code-style bordered box.
type Header struct {
	styles       Styles
	width        int
	modelName    string
	sessionID    string
	skillCount   int
	cwd          string
	version      string
	updateNotice string
	baseURL      string
	apiKey       string
}

// NewHeader creates a Header component.
func NewHeader(s Styles) *Header {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(cwd, home) {
		cwd = "~" + cwd[len(home):]
	}
	return &Header{styles: s, cwd: cwd, version: "0.1.0"}
}

// SetWidth updates the header width.
func (h *Header) SetWidth(w int) { h.width = w }

// SetModel updates the displayed model name.
func (h *Header) SetModel(name string) { h.modelName = name }

// SetSession updates the displayed session ID.
func (h *Header) SetSession(id string) {
	if len(id) > 8 {
		id = id[:8]
	}
	h.sessionID = id
}

// SetSkillCount updates the displayed skill count.
func (h *Header) SetSkillCount(n int) { h.skillCount = n }

// SetUpdateNotice sets the version update notification text.
func (h *Header) SetUpdateNotice(notice string) { h.updateNotice = notice }

// SetProvider sets the base URL and API key for display (both masked).
func (h *Header) SetProvider(baseURL, apiKey string) {
	h.baseURL = baseURL
	h.apiKey = maskAPIKey(apiKey)
}

// maskAPIKey masks an API key for safe display, showing only first 4 and last 4 chars.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

// detectEnvBaseURL returns the configured base URL from environment.
func detectEnvBaseURL() string {
	if v := os.Getenv("ANTHROPIC_BASE_URL"); v != "" {
		return v
	}
	if v := os.Getenv("OPENAI_BASE_URL"); v != "" {
		return v
	}
	return ""
}

// detectEnvAPIKey returns the configured API key from environment.
func detectEnvAPIKey() string {
	if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
		return v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		return v
	}
	if v := os.Getenv("OPENAI_API_KEY"); v != "" {
		return v
	}
	return ""
}

// View renders the header in a Claude Code-style bordered box layout.
func (h *Header) View() string {
	totalWidth := h.width
	if totalWidth < 60 {
		totalWidth = 80
	}

	borderColor := h.styles.Theme.Primary
	dimStyle := lipgloss.NewStyle().Foreground(h.styles.Theme.FgDim)
	boldStyle := lipgloss.NewStyle().Bold(true).Foreground(h.styles.Theme.Fg)
	primaryStyle := lipgloss.NewStyle().Foreground(borderColor).Bold(true)
	borderStyle := lipgloss.NewStyle().Foreground(borderColor)

	// Inner content width (minus 2 for left/right border chars + 2 for padding)
	innerWidth := totalWidth - 4
	leftWidth := innerWidth * 45 / 100
	if leftWidth < 34 {
		leftWidth = 34
	}
	rightWidth := innerWidth - leftWidth - 3 // 3 for the vertical separator " │ "
	if rightWidth < 20 {
		rightWidth = 20
		leftWidth = innerWidth - rightWidth - 3
	}

	// === Top border with version title ===
	title := fmt.Sprintf(" Saker v%s ", h.version)
	topLeftDash := 2
	topRightDash := totalWidth - len(title) - topLeftDash - 2 // 2 for border corners
	if topRightDash < 2 {
		topRightDash = 2
	}
	topBorder := borderStyle.Render("╭" + strings.Repeat("─", topLeftDash)) +
		primaryStyle.Render(title) +
		borderStyle.Render(strings.Repeat("─", topRightDash) + "╮")

	// === Left column content ===
	var leftLines []string

	// Greeting
	leftLines = append(leftLines, boldStyle.Render("  Welcome back!"))
	leftLines = append(leftLines, "")

	// Mascot
	mascotColor := h.styles.LogoColor
	for _, row := range mascotRows {
		leftLines = append(leftLines, mascotColor.Render(row))
	}
	leftLines = append(leftLines, "")

	// Model + session info
	if h.modelName != "" {
		modelInfo := dimStyle.Render(h.modelName)
		if h.skillCount > 0 {
			modelInfo += dimStyle.Render(fmt.Sprintf(" · %d skills", h.skillCount))
		}
		leftLines = append(leftLines, "  "+modelInfo)
	}

	// Provider info (base URL + API key)
	if h.baseURL != "" || h.apiKey != "" {
		var parts []string
		if h.baseURL != "" {
			parts = append(parts, shortenURL(h.baseURL))
		}
		if h.apiKey != "" {
			parts = append(parts, h.apiKey)
		}
		leftLines = append(leftLines, "  "+dimStyle.Render(strings.Join(parts, " · ")))
	}

	// Working directory
	leftLines = append(leftLines, "  "+dimStyle.Render(h.cwd))

	// === Right column content ===
	var rightLines []string

	// Tips section
	rightLines = append(rightLines, primaryStyle.Render("Tips"))
	rightLines = append(rightLines, dimStyle.Render("Esc to scroll history"))
	rightLines = append(rightLines, dimStyle.Render("/ to search in scroll mode"))
	rightLines = append(rightLines, dimStyle.Render("Ctrl+C to interrupt"))
	rightLines = append(rightLines, dimStyle.Render("/help for all commands"))
	rightLines = append(rightLines, "")

	// Keybindings section
	rightLines = append(rightLines, primaryStyle.Render("Shortcuts"))
	rightLines = append(rightLines, dimStyle.Render("Tab     autocomplete commands"))
	rightLines = append(rightLines, dimStyle.Render("Ctrl+L  clear screen"))
	rightLines = append(rightLines, dimStyle.Render("/new    start new conversation"))

	// Update notice
	if h.updateNotice != "" {
		rightLines = append(rightLines, "")
		updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))
		rightLines = append(rightLines, updateStyle.Render(h.updateNotice))
	}

	// === Compose rows ===
	maxRows := len(leftLines)
	if len(rightLines) > maxRows {
		maxRows = len(rightLines)
	}

	var bodyLines []string
	vSep := borderStyle.Render("│")
	for i := 0; i < maxRows; i++ {
		left := ""
		if i < len(leftLines) {
			left = leftLines[i]
		}
		right := ""
		if i < len(rightLines) {
			right = rightLines[i]
		}

		// Pad left column to fixed width
		leftPadded := padToWidth(left, leftWidth)
		rightPadded := padToWidth(right, rightWidth)

		line := borderStyle.Render("│") + " " + leftPadded + " " + vSep + " " + rightPadded + " " + borderStyle.Render("│")
		bodyLines = append(bodyLines, line)
	}

	// === Bottom border ===
	bottomBorder := borderStyle.Render("╰" + strings.Repeat("─", totalWidth-2) + "╯")

	// Assemble
	var b strings.Builder
	b.WriteString(topBorder + "\n")
	for _, line := range bodyLines {
		b.WriteString(line + "\n")
	}
	b.WriteString(bottomBorder + "\n")

	return b.String()
}

// shortenURL extracts host (+port) from a URL for compact display.
func shortenURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return rawURL
	}
	return u.Host
}

// padToWidth pads (or truncates) a string to exactly the given visible width.
func padToWidth(s string, width int) string {
	visible := visibleLen(s)
	if visible >= width {
		return truncateToWidth(s, width)
	}
	return s + strings.Repeat(" ", width-visible)
}

// visibleLen returns the visible character count (excluding ANSI escape sequences).
func visibleLen(s string) int {
	count := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		count++
	}
	return count
}

// truncateToWidth truncates a string to fit within the given visible width,
// preserving ANSI escape sequences.
func truncateToWidth(s string, width int) string {
	var b strings.Builder
	count := 0
	inEscape := false
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			b.WriteRune(r)
			continue
		}
		if inEscape {
			b.WriteRune(r)
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEscape = false
			}
			continue
		}
		if count >= width {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}

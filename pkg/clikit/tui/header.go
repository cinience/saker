package tui

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
)

// mascotRows is a stylised saker falcon (猎隼) emblem,
// symbolising the keen vision and swift precision of Saker.
var mascotRows = [3]string{
	" ▗▄███▄▖   ",
	" ▐◈ ▼ ◈▌   ",
	"  ▀▄█▄▀    ",
}

// Header renders the top section of the TUI with mascot and project info.
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

// View renders the header with falcon emblem and project info.
// Layout:
//
//	▗▄███▄▖   Saker v0.1.0
//	▐◈ ▼ ◈▌   model-name · session:abc123
//	 ▀▄█▄▀    ~/path/to/cwd
//	           Esc scroll · / search · Ctrl+C interrupt
func (h *Header) View() string {
	mascotColor := h.styles.LogoColor

	titleLine := fmt.Sprintf("%s %s",
		lipgloss.NewStyle().Bold(true).Foreground(h.styles.Theme.Fg).Render("Saker"),
		h.styles.HeaderDim.Render("v"+h.version),
	)

	var modelLine string
	if h.modelName != "" {
		modelLine = h.styles.HeaderDim.Render(h.modelName)
		if h.sessionID != "" {
			modelLine += h.styles.HeaderDim.Render(" · ") +
				lipgloss.NewStyle().Foreground(h.styles.Theme.Muted).Render(h.sessionID)
		}
	}

	var providerLine string
	if h.baseURL != "" || h.apiKey != "" {
		var parts []string
		if h.baseURL != "" {
			parts = append(parts, h.baseURL)
		}
		if h.apiKey != "" {
			parts = append(parts, h.apiKey)
		}
		providerLine = h.styles.HeaderDim.Render(strings.Join(parts, " · "))
	}

	cwdLine := h.styles.HeaderDim.Render(h.cwd)

	infoLines := [3]string{titleLine, modelLine, cwdLine}
	var b strings.Builder
	for i := 0; i < 3; i++ {
		mascot := mascotColor.Render(mascotRows[i])
		info := infoLines[i]
		b.WriteString(fmt.Sprintf(" %s %s\n", mascot, info))
	}

	// Provider info (below mascot, aligned with info column)
	if providerLine != "" {
		b.WriteString(fmt.Sprintf("              %s\n", providerLine))
	}

	// Keybinding hints
	hintStyle := lipgloss.NewStyle().Foreground(h.styles.Theme.Muted)
	hints := hintStyle.Render("           Esc scroll · / search · Ctrl+C interrupt")
	b.WriteString(hints + "\n")

	if h.updateNotice != "" {
		updateStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFAA00"))
		b.WriteString(" " + updateStyle.Render(h.updateNotice) + "\n")
	}

	return b.String()
}

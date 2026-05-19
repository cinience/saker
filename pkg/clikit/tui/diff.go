package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderUnifiedDiff renders a unified diff with syntax coloring for the TUI.
func RenderUnifiedDiff(diff string, width int, styles Styles) string {
	if diff == "" {
		return ""
	}

	lines := strings.Split(diff, "\n")
	var rendered []string

	addStyle := lipgloss.NewStyle().Foreground(styles.Theme.Success)
	delStyle := lipgloss.NewStyle().Foreground(styles.Theme.Error)
	hunkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#00BCD4")).Faint(true)
	metaStyle := lipgloss.NewStyle().Foreground(styles.Theme.FgDim)
	normalStyle := lipgloss.NewStyle().Foreground(styles.Theme.Fg)

	for _, line := range lines {
		if width > 0 && len(line) > width-2 {
			line = line[:width-5] + "..."
		}

		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			rendered = append(rendered, metaStyle.Render(line))
		case strings.HasPrefix(line, "@@"):
			rendered = append(rendered, hunkStyle.Render(line))
		case strings.HasPrefix(line, "+"):
			rendered = append(rendered, addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			rendered = append(rendered, delStyle.Render(line))
		default:
			rendered = append(rendered, normalStyle.Render(line))
		}
	}

	return strings.Join(rendered, "\n")
}

// RenderFilePreview renders a file content preview with line numbers.
func RenderFilePreview(content string, maxLines int, styles Styles) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
	}

	numStyle := lipgloss.NewStyle().Foreground(styles.Theme.Muted)
	textStyle := lipgloss.NewStyle().Foreground(styles.Theme.Fg)

	var rendered []string
	for i, line := range lines {
		num := numStyle.Render(strings.Repeat(" ", 3-len(strings.TrimSpace(string(rune('0'+i/10))))+1) + string(rune('0'+((i+1)/10)%10)) + string(rune('0'+(i+1)%10)))
		rendered = append(rendered, num+" "+textStyle.Render(line))
	}

	if maxLines > 0 && len(strings.Split(content, "\n")) > maxLines {
		rendered = append(rendered, styles.SystemText.Render("  ... (truncated)"))
	}

	return strings.Join(rendered, "\n")
}

// RenderBashCommand renders a bash command with basic syntax highlighting.
func RenderBashCommand(cmd string, styles Styles) string {
	if cmd == "" {
		return ""
	}

	keywords := map[string]bool{
		"if": true, "then": true, "else": true, "fi": true,
		"for": true, "do": true, "done": true, "while": true,
		"case": true, "esac": true, "in": true,
		"export": true, "local": true, "return": true,
		"cd": true, "echo": true, "exit": true,
	}

	kwStyle := lipgloss.NewStyle().Foreground(styles.Theme.Secondary).Bold(true)
	flagStyle := lipgloss.NewStyle().Foreground(styles.Theme.FgDim)
	normalStyle := lipgloss.NewStyle().Foreground(styles.Theme.Fg)

	words := strings.Fields(cmd)
	var rendered []string
	for _, w := range words {
		switch {
		case keywords[w]:
			rendered = append(rendered, kwStyle.Render(w))
		case strings.HasPrefix(w, "-"):
			rendered = append(rendered, flagStyle.Render(w))
		default:
			rendered = append(rendered, normalStyle.Render(w))
		}
	}

	return strings.Join(rendered, " ")
}

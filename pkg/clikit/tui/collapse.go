package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// CollapsedGroup represents a group of consecutive tool calls that can be collapsed.
type CollapsedGroup struct {
	ToolName string
	Count    int
	Items    []string
	Status   string // "pending" / "success" / "mixed"
	Expanded bool
	StartIdx int
	EndIdx   int
}

// collapsibleKey returns a normalised key for collapsing consecutive tool
// calls. Tools whose names contain the same keyword are grouped together.
// Returns "" if the tool should not be collapsed.
func collapsibleKey(name string) string {
	lower := strings.ToLower(name)
	for _, kw := range []string{"read", "edit", "write", "bash", "grep", "glob", "lsp", "web_fetch", "webfetch", "web_search", "websearch"} {
		if strings.Contains(lower, kw) {
			return kw
		}
	}
	return ""
}

// CollapseMessages scans messages and groups consecutive collapsible tool calls.
func CollapseMessages(messages []ChatMsg) []CollapsedGroup {
	var groups []CollapsedGroup
	i := 0
	for i < len(messages) {
		msg := messages[i]
		key := ""
		if msg.Role == RoleTool {
			key = collapsibleKey(msg.ToolName)
		}
		if key == "" {
			i++
			continue
		}

		group := CollapsedGroup{
			ToolName: msg.ToolName,
			StartIdx: i,
			Status:   msg.ToolStatus,
		}
		group.Items = append(group.Items, msg.ToolParams)
		group.Count = 1
		j := i + 1

		for j < len(messages) && messages[j].Role == RoleTool && collapsibleKey(messages[j].ToolName) == key {
			group.Items = append(group.Items, messages[j].ToolParams)
			group.Count++
			if messages[j].ToolStatus != group.Status {
				group.Status = "mixed"
			}
			j++
		}

		group.EndIdx = j - 1

		if group.Count >= 2 {
			groups = append(groups, group)
		}
		i = j
	}
	return groups
}

// RenderCollapsedGroup renders a collapsed group summary line.
func RenderCollapsedGroup(g CollapsedGroup, styles Styles) string {
	var statusIcon string
	var statusStyle lipgloss.Style

	switch g.Status {
	case "success":
		statusIcon = IconCheck
		statusStyle = styles.ToolSuccess
	case "error":
		statusIcon = IconError
		statusStyle = styles.ToolError
	case "pending":
		statusIcon = IconPending
		statusStyle = styles.ToolPending
	default:
		statusIcon = IconCheck
		statusStyle = styles.ToolSuccess
	}

	icon := statusStyle.Render(statusIcon)

	var summary string
	switch {
	case len(g.Items) <= 3:
		summary = strings.Join(g.Items, ", ")
	default:
		summary = fmt.Sprintf("%s, %s, +%d", g.Items[0], g.Items[1], len(g.Items)-2)
	}

	verb := toolGroupVerb(g.ToolName)
	dimStyle := lipgloss.NewStyle().Foreground(styles.Theme.FgDim)

	return fmt.Sprintf("  %s %s %d %s %s",
		IconResponse,
		icon,
		g.Count,
		verb,
		dimStyle.Render("("+summary+")"),
	)
}

func toolGroupVerb(name string) string {
	key := collapsibleKey(name)
	switch key {
	case "read":
		return "files read"
	case "edit":
		return "edits"
	case "write":
		return "files written"
	case "bash":
		return "commands run"
	case "grep":
		return "searches"
	case "glob":
		return "globs"
	case "lsp":
		return "lookups"
	case "web_fetch", "webfetch":
		return "pages fetched"
	case "web_search", "websearch":
		return "searches"
	default:
		return "calls"
	}
}

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

// collapsibleTools lists tools that collapse when consecutive.
var collapsibleTools = map[string]bool{
	"Read": true,
	"Grep": true,
	"Glob": true,
}

// CollapseMessages scans messages and groups consecutive collapsible tool calls.
func CollapseMessages(messages []ChatMsg) []CollapsedGroup {
	var groups []CollapsedGroup
	i := 0
	for i < len(messages) {
		msg := messages[i]
		if msg.Role != RoleTool || !collapsibleTools[msg.ToolName] {
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

		for j < len(messages) && messages[j].Role == RoleTool && messages[j].ToolName == msg.ToolName {
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
	switch name {
	case "Read":
		return "files read"
	case "Grep":
		return "searches"
	case "Glob":
		return "globs"
	default:
		return "calls"
	}
}

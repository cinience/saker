package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// View implements tea.Model.
func (a *App) View() tea.View {
	// Permission panel overlay (highest priority).
	if a.permPanel != nil {
		return a.viewPanel(a.permPanel.View())
	}

	// Question panel overlay (interactive AskUserQuestion).
	if a.questionPanel != nil {
		return a.viewPanel(a.questionPanel.View())
	}

	// Side panel overlay.
	if a.sidePanel != nil {
		return a.viewSidePanel()
	}

	// Normal inline layout: live content + spinner + input + status.
	return a.viewNormal()
}

// viewNormal renders the inline layout (no alt-screen).
// Completed messages are flushed via tea.Println into terminal scrollback.
// Only the live streaming area + input + status appear in the re-rendered region.
func (a *App) viewNormal() tea.View {
	var parts []string

	// Live streaming content (unflushed messages + streaming buffer)
	liveContent := a.chat.View()
	if liveContent != "" {
		parts = append(parts, liveContent)
	}

	if a.spinning {
		spinnerView := a.styles.StatusText.Render(a.smartSpinner.View())
		parts = append(parts, spinnerView)
	}

	// Notification bar (above input)
	if notif := a.notifications.View(); notif != "" {
		parts = append(parts, notif)
	}

	// Search bar (in scroll/search mode — only useful if scrollback is in viewport)
	if a.search.IsActive() {
		parts = append(parts, a.search.StatusView(a.width))
	}

	inputView := a.input.View()
	statusView := a.status.View()
	parts = append(parts, inputView, statusView)

	view := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Append CSI ED (Erase in Display, mode 0) to clear from cursor to end
	// of screen. In bubbletea inline mode, when the live area shrinks (e.g.
	// streaming finishes), cursor-up may not reach lines that scrolled past
	// the terminal top, leaving "ghost" content below the new shorter view.
	// \x1b[0J wipes those residual lines every frame.
	view += "\x1b[0J"

	return tea.NewView(view)
}

// viewPanel renders an overlay panel (permission/question) inline.
func (a *App) viewPanel(panelView string) tea.View {
	statusView := a.status.View()

	// Limit panel height to avoid excessive rendering.
	maxH := a.height - 2
	if maxH < 5 {
		maxH = 20
	}
	panelLines := strings.Split(panelView, "\n")
	if len(panelLines) > maxH {
		panelLines = panelLines[:maxH]
	}
	clipped := strings.Join(panelLines, "\n")

	view := lipgloss.JoinVertical(lipgloss.Left, clipped, statusView)
	return tea.NewView(view)
}

// viewSidePanel renders the side panel overlay inline.
func (a *App) viewSidePanel() tea.View {
	panelView := a.sidePanel.View()
	statusView := a.status.View()

	if a.sidePanel.IsInteractive() {
		inputView := a.input.View()
		var parts []string
		parts = append(parts, panelView)
		if a.spinning {
			spinnerView := a.styles.StatusText.Render(a.smartSpinner.View())
			parts = append(parts, spinnerView)
		}
		parts = append(parts, inputView, statusView)
		view := lipgloss.JoinVertical(lipgloss.Left, parts...)
		return tea.NewView(view)
	}

	view := lipgloss.JoinVertical(lipgloss.Left, panelView, statusView)
	return tea.NewView(view)
}

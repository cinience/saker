package tui

import (
	tea "charm.land/bubbletea/v2"
)

// Init implements tea.Model.
func (a *App) Init() tea.Cmd {
	return a.input.textarea.Focus()
}

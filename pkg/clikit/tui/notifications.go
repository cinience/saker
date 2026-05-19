package tui

import (
	"image/color"
	"time"

	"charm.land/lipgloss/v2"
)

// NotificationPriority determines display precedence.
type NotificationPriority int

const (
	PriorityLow NotificationPriority = iota
	PriorityMedium
	PriorityHigh
	PriorityImmediate
)

const defaultNotificationTimeout = 8 * time.Second

// Notification represents a single notification message.
type Notification struct {
	Key      string
	Text     string
	Color    color.Color
	Priority NotificationPriority
	Timeout  time.Duration
	Created  time.Time
}

// NotificationManager manages a priority queue of notifications.
type NotificationManager struct {
	current *Notification
	queue   []Notification
	styles  Styles
}

// NewNotificationManager creates a new notification manager.
func NewNotificationManager(styles Styles) *NotificationManager {
	return &NotificationManager{styles: styles}
}

// Push adds a notification. Higher priority replaces current immediately.
// Same-key notifications update in place.
func (nm *NotificationManager) Push(n Notification) {
	if n.Timeout == 0 {
		n.Timeout = defaultNotificationTimeout
	}
	if n.Created.IsZero() {
		n.Created = time.Now()
	}

	// Same-key merge
	if nm.current != nil && nm.current.Key == n.Key {
		nm.current.Text = n.Text
		nm.current.Color = n.Color
		nm.current.Created = n.Created
		nm.current.Timeout = n.Timeout
		return
	}
	for i := range nm.queue {
		if nm.queue[i].Key == n.Key {
			nm.queue[i] = n
			return
		}
	}

	// Higher priority replaces current
	if nm.current != nil && n.Priority > nm.current.Priority {
		nm.queue = append(nm.queue, *nm.current)
		nm.current = &n
		return
	}

	if nm.current == nil {
		nm.current = &n
		return
	}

	nm.queue = append(nm.queue, n)
}

// Tick removes expired notifications and promotes next in queue.
func (nm *NotificationManager) Tick() {
	if nm.current != nil {
		if time.Since(nm.current.Created) > nm.current.Timeout {
			nm.current = nil
		}
	}

	if nm.current == nil && len(nm.queue) > 0 {
		// Find highest priority in queue
		bestIdx := 0
		for i := 1; i < len(nm.queue); i++ {
			if nm.queue[i].Priority > nm.queue[bestIdx].Priority {
				bestIdx = i
			}
		}
		nm.current = &nm.queue[bestIdx]
		nm.queue = append(nm.queue[:bestIdx], nm.queue[bestIdx+1:]...)
	}
}

// Dismiss removes the current notification.
func (nm *NotificationManager) Dismiss() {
	nm.current = nil
}

// View renders the current notification (empty string if none).
func (nm *NotificationManager) View() string {
	nm.Tick()
	if nm.current == nil {
		return ""
	}

	c := nm.current.Color
	if c == nil {
		c = nm.styles.Theme.FgDim
	}

	style := lipgloss.NewStyle().Foreground(c)
	return style.Render(nm.current.Text)
}

// HasNotification returns true if there's an active notification.
func (nm *NotificationManager) HasNotification() bool {
	nm.Tick()
	return nm.current != nil
}

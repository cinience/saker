package tui

import (
	"fmt"
	"image/color"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/saker-ai/saker/pkg/textutil"
)

const (
	stallThreshold    = 3 * time.Second
	stallFadeDuration = 2 * time.Second
	shimmerWidth      = 3
	shimmerInterval   = 50 * time.Millisecond
	tokenShowDelay    = 10 * time.Second
)

// SmartSpinner wraps bubbletea's spinner with shimmer animation, stall detection,
// token counting, and thinking duration display.
type SmartSpinner struct {
	spinner spinner.Model
	theme   Theme
	styles  Styles
	verb    string
	start   time.Time
	stalled bool
	active  bool

	lastTokenNano atomic.Int64
	charCount     atomic.Int64

	// Shimmer state
	shimmerIdx int
	lastTick   time.Time

	// Reduced motion
	reducedMotion bool
}

func NewSmartSpinner(t Theme, s Styles) *SmartSpinner {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(t.Primary)

	reduced := os.Getenv("SAKER_REDUCED_MOTION") == "1" || os.Getenv("NO_MOTION") == "1"

	return &SmartSpinner{
		spinner:       sp,
		theme:         t,
		styles:        s,
		verb:          "Thinking...",
		reducedMotion: reduced,
	}
}

func (s *SmartSpinner) Start() {
	s.start = time.Now()
	s.lastTokenNano.Store(time.Now().UnixNano())
	s.charCount.Store(0)
	s.stalled = false
	s.active = true
	s.shimmerIdx = 0
	s.lastTick = time.Now()
	s.verb = "Thinking..."
}

func (s *SmartSpinner) Stop() {
	s.active = false
	s.stalled = false
}

func (s *SmartSpinner) SetVerb(v string) {
	s.verb = v
	s.stalled = false
	s.lastTokenNano.Store(time.Now().UnixNano())
}

// AddTokens accumulates character count (safe to call from streaming goroutine).
func (s *SmartSpinner) AddTokens(n int) {
	s.charCount.Add(int64(n))
	s.lastTokenNano.Store(time.Now().UnixNano())
}

func (s *SmartSpinner) CheckStall() {
	if !s.active {
		return
	}
	lastNano := s.lastTokenNano.Load()
	if lastNano == 0 {
		return
	}
	s.stalled = time.Since(time.Unix(0, lastNano)) > stallThreshold
}

func (s *SmartSpinner) Tick() tea.Cmd {
	return s.spinner.Tick
}

func (s *SmartSpinner) Update(msg tea.Msg) tea.Cmd {
	now := time.Now()
	if now.Sub(s.lastTick) >= shimmerInterval {
		s.shimmerIdx++
		s.lastTick = now
	}
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return cmd
}

func (s *SmartSpinner) View() string {
	verb := s.verb
	if s.stalled && !strings.Contains(s.verb, "Running") && !strings.Contains(s.verb, "Generating") {
		verb = "Waiting..."
	}

	elapsed := time.Since(s.start)
	chars := s.charCount.Load()

	// Build stats suffix
	var stats string
	if chars > 0 || elapsed > 2*time.Second {
		var parts []string
		if chars > 0 && elapsed > tokenShowDelay {
			tokens := chars / 4
			parts = append(parts, formatTokenCount(int(tokens))+" tokens")
		} else if chars > 0 {
			parts = append(parts, formatCharCount(int(chars)))
		}
		if elapsed > 2*time.Second {
			parts = append(parts, formatDuration(elapsed))
		}
		if len(parts) > 0 {
			stats = " (" + strings.Join(parts, ", ") + ")"
		}
	}

	fullText := verb + stats

	if s.reducedMotion {
		return s.viewReducedMotion(fullText)
	}

	if s.stalled {
		return s.viewStalled(fullText)
	}

	return s.viewShimmer(fullText)
}

// viewShimmer renders text with a sweeping highlight animation.
func (s *SmartSpinner) viewShimmer(text string) string {
	runes := []rune(text)
	textLen := len(runes)
	if textLen == 0 {
		return s.spinner.View() + " " + text
	}

	// Shimmer position sweeps left-to-right across the text
	totalWidth := textLen + shimmerWidth*2
	pos := s.shimmerIdx % totalWidth

	dimStyle := lipgloss.NewStyle().Foreground(s.theme.FgDim)
	brightStyle := lipgloss.NewStyle().Foreground(s.theme.Primary)
	normalStyle := lipgloss.NewStyle().Foreground(s.theme.Fg)

	var b strings.Builder
	b.WriteString(s.spinner.View())
	b.WriteString(" ")

	for i, r := range runes {
		dist := abs(i - pos + shimmerWidth)
		if dist <= shimmerWidth {
			// Within shimmer window — bright
			b.WriteString(brightStyle.Render(string(r)))
		} else if s.active && time.Since(s.start) < 500*time.Millisecond {
			// Initial fade-in: all dim
			b.WriteString(dimStyle.Render(string(r)))
		} else {
			b.WriteString(normalStyle.Render(string(r)))
		}
	}

	return b.String()
}

// viewStalled renders with yellow→red gradient based on stall duration.
func (s *SmartSpinner) viewStalled(text string) string {
	lastNano := s.lastTokenNano.Load()
	stallDur := time.Since(time.Unix(0, lastNano)) - stallThreshold
	if stallDur < 0 {
		stallDur = 0
	}

	// Intensity: 0.0 (just stalled, yellow) → 1.0 (fully red)
	intensity := float64(stallDur) / float64(stallFadeDuration)
	if intensity > 1.0 {
		intensity = 1.0
	}

	stallColor := lerpColor(s.theme.Warning, s.theme.Error, intensity)
	style := lipgloss.NewStyle().Foreground(stallColor)
	icon := style.Render("●")

	return icon + " " + style.Render(text)
}

// viewReducedMotion renders a static indicator without animation.
func (s *SmartSpinner) viewReducedMotion(text string) string {
	if s.stalled {
		style := lipgloss.NewStyle().Foreground(s.theme.Warning)
		return style.Render("● " + text)
	}
	style := lipgloss.NewStyle().Foreground(s.theme.Primary)
	return style.Render("● " + text)
}

// lerpColor linearly interpolates between two colors.
func lerpColor(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()

	rr := uint8((float64(ar>>8) + t*(float64(br>>8)-float64(ar>>8))))
	rg := uint8((float64(ag>>8) + t*(float64(bg>>8)-float64(ag>>8))))
	rb := uint8((float64(ab>>8) + t*(float64(bb>>8)-float64(ab>>8))))

	return lipgloss.Color(fmt.Sprintf("#%02X%02X%02X", rr, rg, rb))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func toolVerb(name, params string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "bash"):
		if params != "" {
			display := params
			if len(display) > 40 {
				display = textutil.TruncateRunesWithin(display, 40, "...")
			}
			return fmt.Sprintf("Running `%s`...", display)
		}
		return "Running command..."
	case strings.Contains(lower, "read"):
		if params != "" {
			return fmt.Sprintf("Reading %s...", params)
		}
		return "Reading file..."
	case strings.Contains(lower, "write"):
		if params != "" {
			return fmt.Sprintf("Writing %s...", params)
		}
		return "Writing file..."
	case strings.Contains(lower, "edit"):
		if params != "" {
			return fmt.Sprintf("Editing %s...", params)
		}
		return "Editing file..."
	case strings.Contains(lower, "grep"):
		if params != "" {
			return fmt.Sprintf("Searching `%s`...", params)
		}
		return "Searching..."
	case strings.Contains(lower, "glob"):
		return "Finding files..."
	case lower == "generate_image":
		return "Generating image..."
	case lower == "edit_image":
		return "Editing image..."
	case lower == "generate_video":
		return "Generating video..."
	case lower == "generate_3d":
		return "Generating 3D model..."
	case lower == "generate_music":
		return "Generating music..."
	case lower == "text_to_speech":
		return "Generating speech..."
	case lower == "transcribe_audio":
		return "Transcribing audio..."
	default:
		return fmt.Sprintf("Running %s...", name)
	}
}

func formatCharCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM chars", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk chars", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d chars", n)
	}
}

func formatDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	rem := secs % 60
	if rem == 0 {
		return fmt.Sprintf("%dm", mins)
	}
	return fmt.Sprintf("%dm%ds", mins, rem)
}


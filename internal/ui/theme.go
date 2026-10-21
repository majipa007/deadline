// Package ui holds the Bubble Tea models and all rendering.
package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// Palette. AdaptiveColor keeps the board legible on light terminals too.
var (
	ColText   = lipgloss.AdaptiveColor{Light: "#1f2430", Dark: "#e6e9ef"}
	ColMuted  = lipgloss.AdaptiveColor{Light: "#6b7280", Dark: "#7a8290"}
	ColBorder = lipgloss.AdaptiveColor{Light: "#c8ccd4", Dark: "#3a4150"}
	ColAccent = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}

	colTodo    = lipgloss.AdaptiveColor{Light: "#3b6ea5", Dark: "#7aa2f7"}
	colDoing   = lipgloss.AdaptiveColor{Light: "#b06f00", Dark: "#e0af68"}
	colBlocked = lipgloss.AdaptiveColor{Light: "#b02a37", Dark: "#f7768e"}
	colDone    = lipgloss.AdaptiveColor{Light: "#2f7a4f", Dark: "#9ece6a"}
)

// AccentFor is a column's colour. Dev columns map by role: in-progress is
// amber, the terminal column green.
func AccentFor(s task.Status) lipgloss.AdaptiveColor {
	switch s {
	case task.StatusDoing, task.StatusInDev, task.StatusTestingReview:
		return colDoing
	case task.StatusBlocked:
		return colBlocked
	case task.StatusDone, task.StatusShipped:
		return colDone
	}
	return colTodo
}

var (
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColAccent)
	MutedStyle = lipgloss.NewStyle().Foreground(ColMuted)
	HelpStyle  = lipgloss.NewStyle().Foreground(ColMuted).PaddingTop(1)

	ColumnStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 1)

	ColumnFocusedStyle = ColumnStyle.Copy().BorderForeground(ColAccent)

	// Every card state spends two columns on the left (bar + space, or two
	// spaces), so text and countdowns never shift as the selection moves.
	CardStyle = lipgloss.NewStyle().
			Foreground(ColText).
			PaddingLeft(2)

	CardSelectedStyle = CardStyle.Copy().
				Bold(true).
				Foreground(ColAccent).
				BorderStyle(lipgloss.Border{Left: "▌"}).
				BorderLeft(true).
				BorderForeground(ColAccent).
				PaddingLeft(1)

	// CardRelatedStyle marks the selected card's family: same geometry as
	// the selection, with a quiet bar.
	CardRelatedStyle = CardStyle.Copy().
				BorderStyle(lipgloss.NormalBorder()).
				BorderLeft(true).
				BorderForeground(ColMuted).
				PaddingLeft(1)

	CardGrabbedStyle = CardSelectedStyle.Copy().
				Foreground(colDoing).
				BorderForeground(colDoing)

	StatTileStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColBorder).
			Padding(0, 2).
			Align(lipgloss.Center)
)

// ErrorStyle styles failures on every screen; errorLine adds the "! ".
var ErrorStyle = lipgloss.NewStyle().Bold(true).Foreground(colBlocked)

func errorLine(msg string) string { return ErrorStyle.Render("! " + msg) }

// statusTag renders a transient board state (Moving, Family) in the footer,
// amber like the grabbed card.
func statusTag(s string) string { return lipgloss.NewStyle().Bold(true).Foreground(colDoing).Render(s) }

// keyHint renders one footer action: an accent key and a muted verb.
func keyHint(key, action string) string {
	return lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Render(key) +
		MutedStyle.Render(" "+action)
}

// globalHints are the keys that work on every page.
func globalHints() []string {
	return []string{keyHint("tab", "Switch"), keyHint("?", "Help"), keyHint("q", "Quit")}
}

// hintBar wraps footer items between items so no line exceeds width; a
// single item wider than width is clipped.
func hintBar(width int, items ...string) string {
	if width <= 0 {
		width = 80
	}
	clip := lipgloss.NewStyle().MaxWidth(width)
	var lines []string
	line := ""
	for _, it := range items {
		it = clip.Render(it)
		if line != "" && lipgloss.Width(line)+2+lipgloss.Width(it) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += "  "
		}
		line += it
	}
	return strings.Join(append(lines, line), "\n")
}

// pageFrame fills a non-board page below the tab bar with the footer pinned
// to the bottom. An oversized body is clipped so the footer stays visible.
func pageFrame(width, height int, body string, hints ...string) string {
	if width <= 0 {
		width = 80
	}
	footer := hintBar(width, hints...)
	if height <= 0 {
		return body + "\n\n" + footer
	}
	room := height - 2 - lipgloss.Height(footer) // tab bar, blank above footer
	if room < 0 {
		room = 0
	}
	lines := strings.Split(lipgloss.NewStyle().MaxWidth(width).Render(body), "\n")
	if len(lines) > room {
		lines = lines[:room]
	}
	for len(lines) < room {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n\n" + footer
}

// HeaderStyle is the per-column heading, tinted with that column's accent.
func HeaderStyle(s task.Status) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s))
}

// FormatDate renders Singapore-style DD/MM/YYYY.
func FormatDate(t time.Time) string { return t.Format("02/01/2006") }

// minDuration negates to itself (int64 overflow), so it needs nudging.
const minDuration = time.Duration(-1 << 63)

// FormatDuration renders "12m", "3h" or "1d 6h". Negative durations (bad
// timestamps) get a "-" prefix rather than being hidden.
func FormatDuration(d time.Duration) string {
	if d < 0 {
		if d == minDuration {
			d = minDuration + 1
		}
		return "-" + FormatDuration(-d)
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	if hours == 0 {
		return fmt.Sprintf("%dd", days)
	}
	return fmt.Sprintf("%dd %dh", days, hours)
}

// Deadline palette. Overdue shares urgent red; the ✗ marker sets it apart.
var (
	colDeadlineFuture = colDone    // green: plenty of time
	colDeadlineSoon   = colDoing   // amber: 2-3 days
	colDeadlineUrgent = colBlocked // red: today or tomorrow, or passed
)

// UrgencyColor is the colour a deadline renders in at each urgency level.
func UrgencyColor(u task.Urgency) lipgloss.AdaptiveColor {
	switch u {
	case task.UrgencySoon:
		return colDeadlineSoon
	case task.UrgencyUrgent, task.UrgencyOverdue:
		return colDeadlineUrgent
	case task.UrgencyFuture:
		return colDeadlineFuture
	}
	return ColMuted // UrgencyDone and UrgencyNone
}

// RenderDeadline is the card's coloured "● DD/MM/YYYY" line, with ✗ when
// overdue; empty without a deadline.
func RenderDeadline(t task.Task, now time.Time, done task.Status) string {
	if t.Deadline == nil {
		return ""
	}
	u := task.DeadlineUrgency(t, now, done)
	// Format in now's zone, as DeadlineUrgency does, so date and colour agree.
	line := "● " + FormatDate(t.Deadline.In(now.Location()))
	if u == task.UrgencyOverdue {
		line += " ✗"
	}
	return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render(line)
}

// Countdown is a card's deadline at a glance: "late 3d", "today", "2d", in
// its urgency colour, bold once it is pressing. Empty without a deadline or
// once the task is done.
func Countdown(t task.Task, now time.Time, done task.Status) string {
	u := task.DeadlineUrgency(t, now, done)
	days, ok := task.DaysUntilDeadline(t, now)
	if !ok || u == task.UrgencyDone {
		return ""
	}
	text := fmt.Sprintf("%dd", days)
	switch {
	case days < 0:
		text = fmt.Sprintf("late %dd", -days)
	case days == 0:
		text = "today"
	}
	return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Bold(u >= task.UrgencySoon).Render(text)
}

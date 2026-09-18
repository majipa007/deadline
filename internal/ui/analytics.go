package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/stats"
	"gotodo/internal/task"
)

const (
	throughputDays = 14
	// streakMonths is the most month grids the streak calendar shows; six
	// fill a wide terminal, and narrow ones show fewer, down to one.
	streakMonths = 6
	blockedTopN  = 5
)

// AnalyticsModel is the stats page. It has no keys of its own.
type AnalyticsModel struct {
	board *task.Board
	now   func() time.Time

	width  int
	height int
}

// NewAnalyticsModel wires a board into the analytics page.
func NewAnalyticsModel(b *task.Board) AnalyticsModel {
	return AnalyticsModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *AnalyticsModel) SetSize(w, h int) { m.width, m.height = w, h }

// View renders the whole analytics page.
func (m AnalyticsModel) View() string {
	now := m.now()
	tasks := m.board.Tasks

	sections := []string{
		m.renderTiles(m.board.Active()),
		m.renderThroughput(tasks, now),
		m.renderCycle(tasks, now),
		m.renderBlocked(m.board.Active(), now),
		m.renderStreak(tasks, now),
	}
	return strings.Join(sections, "\n\n")
}

func (m AnalyticsModel) renderTiles(tasks []task.Task) string {
	counts := stats.Counts(tasks, m.board.Statuses())
	tiles := make([]string, 0, len(m.board.Statuses()))
	for _, s := range m.board.Statuses() {
		body := lipgloss.JoinVertical(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(AccentFor(s)).
				Render(fmt.Sprintf("%d", counts[s])),
			MutedStyle.Render(s.Label()),
		)
		tiles = append(tiles, StatTileStyle.Render(body))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tiles...)
}

func (m AnalyticsModel) renderThroughput(tasks []task.Task, now time.Time) string {
	series := stats.Throughput(tasks, throughputDays, now, m.board.DoneStatus())
	values := make([]int, len(series))
	total := 0
	for i, d := range series {
		values[i] = d.N
		total += d.N
	}

	line := lipgloss.NewStyle().Foreground(AccentFor(m.board.DoneStatus())).
		Render(Sparkline(values))

	span := ""
	if len(series) > 0 {
		span = fmt.Sprintf("%s → %s",
			FormatDate(series[0].Day), FormatDate(series[len(series)-1].Day))
	}

	return strings.Join([]string{
		TitleStyle.Render("THROUGHPUT"),
		line,
		MutedStyle.Render(fmt.Sprintf("%d completed over %d days · %s",
			total, throughputDays, span)),
	}, "\n")
}

func (m AnalyticsModel) renderCycle(tasks []task.Task, now time.Time) string {
	c := stats.CycleTimes(tasks, now, m.board.DoneStatus())
	lines := []string{TitleStyle.Render("CYCLE TIME")}

	if c.N == 0 {
		return strings.Join(append(lines, MutedStyle.Render("no completed tasks yet")), "\n")
	}

	lines = append(lines, fmt.Sprintf("mean %s · median %s · over %d completed",
		FormatDuration(c.Mean), FormatDuration(c.Median), c.N))
	lines = append(lines, MutedStyle.Render("mean time spent per column:"))

	maxMinutes := 0
	for _, s := range m.board.Statuses() {
		if v := int(c.PerColumn[s].Minutes()); v > maxMinutes {
			maxMinutes = v
		}
	}
	barWidth := m.barWidth()
	for _, s := range m.board.Statuses() {
		d := c.PerColumn[s]
		bar := HBar(strings.ToLower(s.Label()), int(d.Minutes()), maxMinutes, barWidth)
		// Replace the raw minute count with a human duration.
		bar = strings.TrimSuffix(bar, fmt.Sprintf(" %d", int(d.Minutes())))
		lines = append(lines, lipgloss.NewStyle().Foreground(AccentFor(s)).Render(bar)+
			" "+MutedStyle.Render(FormatDuration(d)))
	}
	return strings.Join(lines, "\n")
}

// barWidth stretches the cycle-time bars across the terminal instead of a
// fixed 24 cells, so the section grows with the board. An unknown width
// keeps the old fixed size.
func (m AnalyticsModel) barWidth() int {
	if m.width <= 0 {
		return 24
	}
	if w := m.width - 32; w > 24 {
		return w
	}
	return 24
}

func (m AnalyticsModel) renderBlocked(tasks []task.Task, now time.Time) string {
	items := stats.BlockedReport(tasks, now)
	lines := []string{TitleStyle.Render("BLOCKED")}

	if len(items) == 0 {
		return strings.Join(append(lines, MutedStyle.Render("nothing is blocked")), "\n")
	}
	lines = append(lines, fmt.Sprintf("%d currently blocked", len(items)))

	n := len(items)
	if n > blockedTopN {
		n = blockedTopN
	}
	for _, it := range items[:n] {
		lines = append(lines, fmt.Sprintf("  %-40s %s",
			truncate(it.Title, 40),
			lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
				Render(FormatDuration(it.For))))
	}
	if len(items) > n {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("  … and %d more", len(items)-n)))
	}
	return strings.Join(lines, "\n")
}

// renderStreak shows the run of days you finished something on real month
// grids rather than an anonymous 12-week strip: the same span, but every
// mark sits on a date you can name.
func (m AnalyticsModel) renderStreak(tasks []task.Task, now time.Time) string {
	current, longest := stats.Streak(tasks, now, m.board.DoneStatus())
	byDay := stats.CompletionsByDay(tasks, now.Location(), m.board.DoneStatus())

	n := monthsAcross(m.width, streakMonths)
	first := monthStart(now).AddDate(0, -(n - 1), 0)

	// Scale the shading against the busiest day on screen, so a single
	// completion still reads as a mark rather than as almost-nothing.
	busiest := 0
	for day, c := range byDay {
		if !day.Before(first) && c > busiest {
			busiest = c
		}
	}

	return strings.Join([]string{
		TitleStyle.Render("STREAK"),
		fmt.Sprintf("current %s · longest %s", plural(current, "day"), plural(longest, "day")),
		monthStrip(first, n, completionCell(byDay, busiest, now)),
		MutedStyle.Render(fmt.Sprintf("%s%s%s%s more finished that day · underline is today, %s",
			string(heatRunes[1]), string(heatRunes[2]), string(heatRunes[3]), string(heatRunes[4]),
			FormatDate(now))),
	}, "\n")
}

// plural renders a count with its unit, adding an s unless the count is one:
// "1 day", "3 days", "0 days".
func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

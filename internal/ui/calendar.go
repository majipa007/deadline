package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/stats"
	"gotodo/internal/task"
)

// monthCell returns one day's text and style. The text must be exactly four
// columns wide or the grid misaligns with its header.
type monthCell func(day time.Time) (string, lipgloss.Style)

// monthGap is the blank gutter between two month grids joined side by side.
const monthGap = 3

// monthView renders one month as a Monday-first grid. cell owns each day's
// look, so the date picker and both calendars share this layout.
func monthView(month time.Time, cell monthCell) string {
	title := month.Format("January 2006")
	pad := (pickerWidth - len(title)) / 2
	if pad < 0 {
		pad = 0
	}

	rows := []string{
		strings.Repeat(" ", pad) + TitleStyle.Render(title),
		MutedStyle.Render(" Mo  Tu  We  Th  Fr  Sa  Su "),
	}
	for _, week := range monthGrid(month) {
		var b strings.Builder
		blank := true
		for _, day := range week {
			if day.IsZero() {
				b.WriteString("    ")
				continue
			}
			blank = false
			text, style := cell(day)
			b.WriteString(style.Render(text))
		}
		if blank {
			continue // a wholly empty trailing week
		}
		rows = append(rows, b.String())
	}
	return strings.Join(rows, "\n")
}

// monthStrip renders n consecutive months from first, side by side.
func monthStrip(first time.Time, n int, cell monthCell) string {
	grids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if i > 0 {
			grids = append(grids, strings.Repeat(" ", monthGap))
		}
		grids = append(grids, monthView(first.AddDate(0, i, 0), cell))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, grids...)
}

// monthStart is the first day of t's month, at midnight in t's location.
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// monthsAcross is how many month grids fit in width, clamped to [1, max].
// An unknown width assumes 80 columns.
func monthsAcross(width, max int) int {
	if width <= 0 {
		width = 80
	}
	n := (width + monthGap) / (pickerWidth + monthGap)
	if n < 1 {
		n = 1
	}
	if n > max {
		n = max
	}
	return n
}

// completionCell shades a day by tasks finished on it, scaled to the busiest
// day. Today is underlined, not recoloured, so green only means "finished".
func completionCell(byDay map[time.Time]int, max int, today time.Time) monthCell {
	return func(day time.Time) (string, lipgloss.Style) {
		n := byDay[task.StartOfDay(day)]
		mark, style := ' ', MutedStyle
		if n > 0 {
			mark = heatRunes[heatLevel(n, max)]
			style = lipgloss.NewStyle().Foreground(AccentFor(task.StatusDone)).Bold(true)
		}
		if sameDay(day, today) {
			style = style.Copy().Underline(true)
		}
		return fmt.Sprintf("%c%2d ", mark, day.Day()), style
	}
}

// deadlineCell bullets each due day in its card's urgency colour; a day with
// several deadlines takes the most pressing one.
func deadlineCell(byDay map[time.Time][]task.Task, today time.Time, done task.Status) monthCell {
	return func(day time.Time) (string, lipgloss.Style) {
		due := byDay[task.StartOfDay(day)]
		text, style := fmt.Sprintf(" %2d ", day.Day()), MutedStyle
		if len(due) > 0 {
			text = fmt.Sprintf("●%2d ", day.Day())
			style = lipgloss.NewStyle().Foreground(UrgencyColor(worstUrgency(due, today, done))).Bold(true)
		}
		if sameDay(day, today) {
			style = style.Copy().Underline(true)
		}
		return text, style
	}
}

// worstUrgency is the most pressing urgency among tasks. Urgency constants
// ascend in pressure, so the largest wins.
func worstUrgency(tasks []task.Task, now time.Time, done task.Status) task.Urgency {
	worst := task.UrgencyNone
	for _, t := range tasks {
		if u := task.DeadlineUrgency(t, now, done); u > worst {
			worst = u
		}
	}
	return worst
}

// calendarMonths is the most months the deadline page shows at once.
const calendarMonths = 6

// upcomingMin is the fewest upcoming-list rows kept on a short terminal, so
// the list can still say how much it hid.
const upcomingMin = 1

// CalendarModel is the view-only deadline page: deadlines on a month strip
// with the upcoming list underneath.
type CalendarModel struct {
	board *task.Board
	now   func() time.Time

	offset int // months away from today's month, moved with h/l
	width  int
	height int
}

// NewCalendarModel wires a board into the deadline calendar page.
func NewCalendarModel(b *task.Board) CalendarModel {
	return CalendarModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *CalendarModel) SetSize(w, h int) { m.width, m.height = w, h }

// Update pages months: h/l step one, t returns to today.
func (m CalendarModel) Update(msg tea.Msg) (CalendarModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "h", "left":
		m.offset--
	case "l", "right":
		m.offset++
	case "t":
		m.offset = 0
	}
	return m, nil
}

// dated is every unfinished task with a deadline, soonest first.
func (m CalendarModel) dated() []task.Task {
	done := m.board.DoneStatus()
	var out []task.Task
	for _, t := range m.board.Active() {
		if t.Status != done {
			out = append(out, t)
		}
	}
	return stats.ByDeadline(out)
}

// View renders the month strip, its legend, and the upcoming list.
func (m CalendarModel) View() string {
	now := m.now()
	tasks := m.dated()
	byDay := stats.DeadlinesByDay(tasks, now.Location())

	n := monthsAcross(m.width, calendarMonths)
	first := monthStart(now).AddDate(0, m.offset, 0)
	strip := monthStrip(first, n, deadlineCell(byDay, now, m.board.DoneStatus()))

	sections := []string{
		TitleStyle.Render("Deadlines"),
		strip,
		MutedStyle.Render("● due   green more than 3 days   amber 3 or fewer   red 1 or fewer, or late   underline today"),
		m.renderUpcoming(tasks, now, strip),
	}
	if m.offset != 0 {
		sections[0] += MutedStyle.Render(fmt.Sprintf("  %+d months from today", m.offset))
	}
	return pageFrame(m.width, m.height, strings.Join(sections, "\n"),
		append([]string{keyHint("h/l", "Month"), keyHint("t", "Today")}, globalHints()...)...)
}

// renderUpcoming lists tasks soonest first (overdue on top) in the rows the
// month strip left free.
func (m CalendarModel) renderUpcoming(tasks []task.Task, now time.Time, strip string) string {
	lines := []string{TitleStyle.Render("Upcoming")}
	if len(tasks) == 0 {
		return strings.Join(append(lines, MutedStyle.Render("nothing has a deadline")), "\n")
	}

	// Rows spent: tab bar, DEADLINES heading, strip, legend, this heading,
	// and pageFrame's two-row footer.
	room := m.height - lipgloss.Height(strip) - 6
	if m.height <= 0 || room > len(tasks) {
		room = len(tasks)
	}
	if room < upcomingMin {
		room = upcomingMin
	}

	shown := tasks
	if len(shown) > room {
		shown = shown[:room-1] // leave a row for the "… and N more" line
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	for _, t := range shown {
		u := task.DeadlineUrgency(t, now, m.board.DoneStatus())
		lines = append(lines, fmt.Sprintf("%s  %-*s %s",
			lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render("● "+FormatDate(t.Deadline.In(now.Location()))),
			max(width-40, 20), truncate(t.Title, max(width-40, 20)),
			MutedStyle.Render(dueIn(t, now))))
	}
	if len(shown) < len(tasks) {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("  … and %d more", len(tasks)-len(shown))))
	}
	return strings.Join(lines, "\n")
}

// dueIn is "today", "tomorrow", "in 6 days" or "4 days late".
func dueIn(t task.Task, now time.Time) string {
	days, ok := task.DaysUntilDeadline(t, now)
	if !ok {
		return ""
	}
	switch {
	case days < 0:
		return plural(-days, "day") + " late"
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	}
	return "in " + plural(days, "day")
}

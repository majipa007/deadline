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

// monthCell decides how one day is drawn: the text of its cell and the style
// to render that text in. Every cell must be exactly four columns wide, or
// the grid stops lining up with its header — see monthView.
type monthCell func(day time.Time) (string, lipgloss.Style)

// monthGap is the blank gutter between two month grids joined side by side.
const monthGap = 3

// monthView renders one month as a Monday-first grid: a centred title, the
// weekday header, and one line per non-empty week. The cell function owns
// everything about a day's appearance, which is what lets the date picker,
// the streak calendar and the deadline calendar share this layout.
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

// monthStrip renders n consecutive months side by side, the first being
// `first`. Shorter months are padded to the tallest, so the strip is one
// rectangular block.
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

// monthsAcross is how many month grids fit in width, at least one and never
// more than max. A width we have not been told yet assumes 80 columns, the
// narrowest terminal the board itself supports.
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

// completionCell shades a day by how many tasks were finished on it: blank
// for none, then the heat runes climbing to a full block, scaled against the
// busiest day on screen. Today is underlined rather than recoloured, so the
// green stays free to mean "you finished something".
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

// deadlineCell marks every day something is due with a bullet, in the same
// urgency colour the card carries: green further out, amber within three
// days, red today, tomorrow or already missed. A day with several deadlines
// takes the colour of its most pressing one.
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

// worstUrgency is the most pressing urgency among tasks. The Urgency
// constants are declared in ascending pressure, so the largest wins.
func worstUrgency(tasks []task.Task, now time.Time, done task.Status) task.Urgency {
	worst := task.UrgencyNone
	for _, t := range tasks {
		if u := task.DeadlineUrgency(t, now, done); u > worst {
			worst = u
		}
	}
	return worst
}

// calendarMonths is the most months the deadline page shows at once; six
// fill a wide terminal the way three fit a 90-column one, which is what
// monthsAcross falls back from.
const calendarMonths = 6

// upcomingMin is how many rows of the upcoming list survive on a short
// terminal. One: even squeezed to nothing the list still says how much it
// could not show. Below about 15 rows the month strip alone is taller than
// the terminal, and nothing here can fix that — the strip is the page.
const upcomingMin = 1

// CalendarModel is the fourth page: every deadline on the board laid out on
// real months, with the list of what is due next underneath. It is view-only
// — nothing here changes a task.
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

// Update pages through the months. h/l step one month, t comes back to
// today's.
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

// dated is every task on the board that has a deadline and is not finished:
// a task in the terminal column is history, and this page is about what is
// coming.
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
		TitleStyle.Render("DEADLINES"),
		strip,
		MutedStyle.Render("● due · green >3 days · amber ≤3 · red ≤1 or overdue · underline is today"),
		m.renderUpcoming(tasks, now, strip),
	}
	if m.offset != 0 {
		sections[0] += MutedStyle.Render(fmt.Sprintf("  %+d months from today", m.offset))
	}
	sections = append(sections, HelpStyle.Render("h/l month · t today · tab switch"))
	return strings.Join(sections, "\n")
}

// renderUpcoming lists what is due next, soonest first, filling whatever rows
// the month strip left behind. Overdue tasks sort in ahead of everything, so
// the thing you have already missed is the first line you read.
func (m CalendarModel) renderUpcoming(tasks []task.Task, now time.Time, strip string) string {
	lines := []string{TitleStyle.Render("UPCOMING")}
	if len(tasks) == 0 {
		return strings.Join(append(lines, MutedStyle.Render("nothing has a deadline")), "\n")
	}

	// Rows already spent: the tab bar, the DEADLINES heading, the strip, the
	// legend, this heading, and the two-row help footer.
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

// dueIn is the human gap to a deadline: "today", "tomorrow", "in 6 days",
// "4 days late".
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

package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// calBoard is a board whose tasks are due the given number of days from ref.
func calBoard(offsets ...int) *task.Board {
	b := &task.Board{}
	b.SetPath("")
	for _, d := range offsets {
		due := ref.AddDate(0, 0, d)
		b.Add("[Task title]", "", &due, ref)
	}
	return b
}

func calendarPage(b *task.Board, w, h int) CalendarModel {
	m := NewCalendarModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(w, h)
	return m
}

// Every cell must be four columns, or the grid stops lining up with the
// weekday header — the invariant monthView's callers all depend on.
func TestMonthViewRowsAreAllTheSameWidth(t *testing.T) {
	out := monthView(ref, func(day time.Time) (string, lipgloss.Style) {
		return " 99 ", lipgloss.NewStyle()
	})
	lines := strings.Split(stripANSI(out), "\n")
	if len(lines) < 4 {
		t.Fatalf("month view has %d lines, want a title, a header and some weeks:\n%s", len(lines), out)
	}
	for i, line := range lines[1:] { // the title is centred, not padded
		if w := lipgloss.Width(line); w != pickerWidth {
			t.Errorf("line %d is %d columns wide, want %d:\n%q", i+1, w, pickerWidth, line)
		}
	}
}

func TestMonthsAcrossFitsTheWidth(t *testing.T) {
	cases := []struct{ width, max, want int }{
		{0, 3, 2},   // unknown width falls back to 80 columns
		{80, 3, 2},  // 2*28 + 3 = 59 fits, 3 would need 90
		{90, 3, 3},  //
		{200, 3, 3}, // capped at max
		{20, 3, 1},  // never fewer than one
	}
	for _, c := range cases {
		if got := monthsAcross(c.width, c.max); got != c.want {
			t.Errorf("monthsAcross(%d, %d) = %d, want %d", c.width, c.max, got, c.want)
		}
	}
}

func TestCalendarMarksDeadlineDays(t *testing.T) {
	// 30/07/2026 is a Thursday. Due today, and in 5 days.
	out := stripANSI(calendarPage(calBoard(0, 5), 110, 40).View())

	if !strings.Contains(out, "July 2026") {
		t.Errorf("calendar does not start on the current month:\n%s", out)
	}
	if !strings.Contains(out, "●30") {
		t.Errorf("today's deadline is not marked:\n%s", out)
	}
	if !strings.Contains(out, "● 4") {
		t.Errorf("the deadline five days out (04/08) is not marked:\n%s", out)
	}
	if strings.Contains(out, "●29") {
		t.Errorf("a day with nothing due was marked:\n%s", out)
	}
}

func TestCalendarColoursDaysByUrgency(t *testing.T) {
	cases := []struct {
		days int
		want task.Urgency
	}{
		{-2, task.UrgencyOverdue},
		{0, task.UrgencyUrgent},
		{1, task.UrgencyUrgent},
		{3, task.UrgencySoon},
		{30, task.UrgencyFuture},
	}
	for _, c := range cases {
		b := calBoard(c.days)
		day := task.StartOfDay(ref.AddDate(0, 0, c.days))
		cell := deadlineCell(map[time.Time][]task.Task{day: b.Tasks}, ref, task.StatusDone)

		_, style := cell(day)
		if got, want := style.GetForeground(), UrgencyColor(c.want); got != want {
			t.Errorf("a deadline %d days out is %v, want the %v colour %v", c.days, got, c.want, want)
		}
	}
}

// A day with several deadlines takes the colour of its most pressing task.
func TestCalendarDayTakesItsMostUrgentDeadline(t *testing.T) {
	day := task.StartOfDay(ref)
	soon := ref.AddDate(0, 0, 3)
	tasks := []task.Task{
		{Title: "[Later]", Status: task.StatusTodo, Deadline: &soon},
		{Title: "[Today]", Status: task.StatusTodo, Deadline: &day},
	}
	if got := worstUrgency(tasks, ref, task.StatusDone); got != task.UrgencyUrgent {
		t.Errorf("worstUrgency = %v, want UrgencyUrgent (the nearer deadline wins)", got)
	}
}

func TestCalendarListsUpcomingOverdueFirst(t *testing.T) {
	out := stripANSI(calendarPage(calBoard(5, -3, 0), 110, 40).View())

	order := []string{"27/07/2026", "30/07/2026", "04/08/2026"}
	at := -1
	for _, want := range order {
		i := strings.Index(out, want)
		if i < 0 {
			t.Fatalf("upcoming list is missing %s:\n%s", want, out)
		}
		if i < at {
			t.Errorf("upcoming list is out of order at %s, want %v soonest first:\n%s", want, order, out)
		}
		at = i
	}
	for _, want := range []string{"3 days late", "today", "in 5 days"} {
		if !strings.Contains(out, want) {
			t.Errorf("upcoming list is missing the gap %q:\n%s", want, out)
		}
	}
}

func TestCalendarSkipsDoneTasksAndTasksWithoutDeadlines(t *testing.T) {
	b := calBoard(2)
	due := ref.AddDate(0, 0, 4)
	doneID := b.Add("[Finished]", "", &due, ref).ID
	if err := b.Move(doneID, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	b.Add("[No deadline]", "", nil, ref)

	out := stripANSI(calendarPage(b, 110, 40).View())
	if strings.Contains(out, "03/08/2026") {
		t.Errorf("a done task's deadline is on the calendar:\n%s", out)
	}
	if !strings.Contains(out, "01/08/2026") {
		t.Errorf("the open task's deadline is missing:\n%s", out)
	}
}

func TestCalendarEmptyState(t *testing.T) {
	out := stripANSI(calendarPage(&task.Board{}, 110, 40).View())
	if !strings.Contains(out, "nothing has a deadline") {
		t.Errorf("empty calendar does not say so:\n%s", out)
	}
	if !strings.Contains(out, "July 2026") {
		t.Errorf("empty calendar still draws the month:\n%s", out)
	}
}

func TestCalendarPagesThroughMonths(t *testing.T) {
	m := calendarPage(calBoard(0), 110, 40)

	m, _ = m.Update(key("l"))
	if m.offset != 1 {
		t.Fatalf("offset = %d, want 1 after l", m.offset)
	}
	if out := stripANSI(m.View()); !strings.Contains(out, "August 2026") || strings.Contains(out, "July 2026") {
		t.Errorf("l did not move the strip to August:\n%s", out)
	}

	m, _ = m.Update(key("h"))
	m, _ = m.Update(key("h"))
	if m.offset != -1 {
		t.Fatalf("offset = %d, want -1 after two h", m.offset)
	}
	m, _ = m.Update(key("t"))
	if m.offset != 0 {
		t.Errorf("offset = %d, want 0 after t", m.offset)
	}
}

// The list is what gives when the terminal is short: the page must stay
// inside the frame however many deadlines there are. 15 rows is the floor
// this can hold — the tab bar, two headings, the legend, the two-row help
// footer and one list row around a strip that is 8 rows tall in a month
// spanning six weeks. Below that the strip alone overflows, and no list
// arithmetic can help.
func TestCalendarPageNeverExceedsTerminalHeight(t *testing.T) {
	offsets := make([]int, 0, 40)
	for i := 0; i < 40; i++ {
		offsets = append(offsets, i)
	}
	for _, h := range []int{15, 16, 18, 22, 30, 40} {
		a := NewApp(calBoard(offsets...))
		a.now = func() time.Time { return ref }
		a.calendar.now = func() time.Time { return ref }
		m, _ := a.Update(tea.WindowSizeMsg{Width: 110, Height: h})
		a = m.(AppModel)
		a.page = pageCalendar

		if got := lipgloss.Height(a.View()); got > h {
			t.Errorf("height=%d: calendar page is %d rows tall, overflows by %d:\n%s", h, got, got-h, a.View())
		}
		if !strings.Contains(stripANSI(a.View()), "… and") {
			t.Errorf("height=%d: 40 deadlines rendered without a '… and N more' line:\n%s", h, a.View())
		}
	}
}

func TestTabReachesTheCalendarPage(t *testing.T) {
	a := app(t)
	a.calendar.now = func() time.Time { return ref }
	for i := 0; i < 3; i++ {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
	}
	if a.page != pageCalendar {
		t.Fatalf("page = %v, want pageCalendar after three tabs", a.page)
	}
	if out := stripANSI(a.View()); !strings.Contains(out, "DEADLINES") {
		t.Errorf("calendar page did not render:\n%s", out)
	}
}

// --- streak calendar ---

// The shading is asserted on the cell function rather than on the rendered
// page: "20" also appears inside "2026" in every month title, so scanning
// the output for a day number matches a heading first.
func TestCompletionCellShadesByCount(t *testing.T) {
	quiet := task.StartOfDay(ref.AddDate(0, 0, -10))
	busy := task.StartOfDay(ref.AddDate(0, 0, -1))
	empty := task.StartOfDay(ref.AddDate(0, 0, -2))
	cell := completionCell(map[time.Time]int{quiet: 1, busy: 4}, 4, ref)

	mark := func(day time.Time) rune {
		text, _ := cell(day)
		return []rune(text)[0]
	}
	if got := mark(busy); got != heatRunes[4] {
		t.Errorf("busiest day is marked %q, want %q", got, heatRunes[4])
	}
	if got := mark(quiet); got == ' ' || got == heatRunes[4] {
		t.Errorf("a one-completion day is marked %q, want a shade below the busiest day's", got)
	}
	if got := mark(empty); got != ' ' {
		t.Errorf("a day with no completions is marked %q, want blank", got)
	}
	// Today is underlined, not recoloured: green stays free to mean
	// "you finished something".
	if _, style := cell(task.StartOfDay(ref)); !style.GetUnderline() {
		t.Error("today is not underlined on the streak calendar")
	}
}

func TestStreakRendersMonthGridsWithCompletions(t *testing.T) {
	b := &task.Board{}
	b.SetPath("")
	for _, d := range []int{-1, -1, -40} {
		day := ref.AddDate(0, 0, d)
		id := b.Add("[Done task]", "", nil, day).ID
		if err := b.Move(id, task.StatusDone, day); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	m := NewAnalyticsModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(110, 40)
	out := stripANSI(m.View())

	for _, want := range []string{"STREAK", "May 2026", "June 2026", "July 2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("streak calendar is missing %q:\n%s", want, out)
		}
	}
	// 29/07 had two completions, the busiest day on screen, so its cell
	// carries the top shade. TestCompletionCellShadesByCount covers the
	// rest of the scale.
	if !strings.Contains(out, string(heatRunes[4])+"29") {
		t.Errorf("the busiest day is not shaded at full strength:\n%s", out)
	}
	if !strings.Contains(out, "current 1 day") {
		t.Errorf("streak counts are missing:\n%s", out)
	}
}

func TestStreakNarrowTerminalShowsFewerMonths(t *testing.T) {
	m := NewAnalyticsModel(&task.Board{})
	m.now = func() time.Time { return ref }
	m.SetSize(80, 40)
	out := stripANSI(m.View())

	if strings.Contains(out, "May 2026") {
		t.Errorf("80 columns fits two months, not three:\n%s", out)
	}
	for _, want := range []string{"June 2026", "July 2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("streak calendar is missing %q at 80 columns:\n%s", want, out)
		}
	}
}

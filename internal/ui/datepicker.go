package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"gotodo/internal/task"
)

// datePicker is the month calendar under the Deadline field. The month shown
// is always the cursor's month.
type datePicker struct {
	// cursor is the selected day, held at noon so DST shifts (which happen
	// near midnight) cannot move it onto another calendar day.
	cursor time.Time
	open   bool
}

// newDatePicker opens a picker on the given day, anchored to noon.
func newDatePicker(seed time.Time) datePicker {
	y, m, d := seed.Date()
	return datePicker{
		cursor: time.Date(y, m, d, 12, 0, 0, 0, seed.Location()),
		open:   true,
	}
}

// move shifts the cursor by whole days, re-anchoring to noon so repeated
// moves never drift.
func (p datePicker) move(days int) datePicker {
	moved := p.cursor.AddDate(0, 0, days)
	y, m, d := moved.Date()
	p.cursor = time.Date(y, m, d, 12, 0, 0, 0, p.cursor.Location())
	return p
}

// sameDay reports whether a and b share a calendar day, each in its own
// location.
func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

// daysInMonth uses day 0 of the next month, which is this month's last day.
func daysInMonth(year int, m time.Month) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// monthGrid lays the cursor's month out as six Monday-first weeks; cells
// outside the month are zero. Six rows fit the worst case (31 days from a
// Sunday, 37 cells).
func monthGrid(cursor time.Time) [6][7]time.Time {
	var g [6][7]time.Time
	loc := cursor.Location()
	first := time.Date(cursor.Year(), cursor.Month(), 1, 0, 0, 0, 0, loc)
	lead := (int(first.Weekday()) + 6) % 7 // Monday-first; Go's Weekday starts at Sunday

	for d := 1; d <= daysInMonth(cursor.Year(), cursor.Month()); d++ {
		i := lead + d - 1
		g[i/7][i%7] = time.Date(cursor.Year(), cursor.Month(), d, 0, 0, 0, 0, loc)
	}
	return g
}

// pickerWidth is the rendered width: seven cells of four columns each.
const pickerWidth = 28

// View renders the picker; today is a parameter so tests stay
// deterministic. Today is green and the cursor is bracketed, possibly both.
func (p datePicker) View(today time.Time) string {
	return monthView(p.cursor, func(day time.Time) (string, lipgloss.Style) {
		text := fmt.Sprintf(" %2d ", day.Day())
		style := lipgloss.NewStyle()
		switch {
		case sameDay(day, today):
			style = style.Foreground(AccentFor(task.StatusDone))
		case sameDay(day, p.cursor):
			style = style.Foreground(ColAccent)
		}
		if sameDay(day, p.cursor) {
			text = fmt.Sprintf("[%2d]", day.Day())
			style = style.Bold(true)
		}
		return text, style
	})
}

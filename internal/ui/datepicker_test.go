package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// day is a terse constructor for a midnight UTC date.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func TestNewDatePickerSeedsOnTheGivenDayAndOpens(t *testing.T) {
	p := newDatePicker(day(2026, time.August, 9))
	if !p.open {
		t.Error("open = false, want true")
	}
	if !sameDay(p.cursor, day(2026, time.August, 9)) {
		t.Errorf("cursor = %v, want 09/08/2026", p.cursor)
	}
}

func TestNewDatePickerAnchorsTheSeedToItsCalendarDay(t *testing.T) {
	p := newDatePicker(time.Date(2026, time.August, 9, 17, 45, 3, 0, time.UTC))
	if !sameDay(p.cursor, day(2026, time.August, 9)) {
		t.Errorf("cursor = %v, want 09/08/2026 regardless of the seed's time of day", p.cursor)
	}
}

func TestDatePickerCursorIsAlwaysHeldAtNoon(t *testing.T) {
	checkNoon := func(t *testing.T, label string, got time.Time) {
		t.Helper()
		if h, m, s := got.Clock(); h != 12 || m != 0 || s != 0 || got.Nanosecond() != 0 {
			t.Errorf("%s cursor = %v, want 12:00:00.000000000", label, got)
		}
	}
	checkNoon(t, "newDatePicker", newDatePicker(time.Date(2026, time.August, 9, 17, 45, 3, 0, time.UTC)).cursor)
	checkNoon(t, "move", newDatePicker(day(2026, time.August, 9)).move(3).cursor)
	checkNoon(t, "move across a month", newDatePicker(day(2026, time.August, 31)).move(1).cursor)
}

func TestMoveByDaysCrossesMonthAndYearBoundaries(t *testing.T) {
	cases := []struct {
		name string
		from time.Time
		by   int
		want time.Time
	}{
		{"forward one day", day(2026, time.August, 9), 1, day(2026, time.August, 10)},
		{"back one day", day(2026, time.August, 9), -1, day(2026, time.August, 8)},
		{"forward one week", day(2026, time.August, 9), 7, day(2026, time.August, 16)},
		{"over the month end", day(2026, time.August, 31), 1, day(2026, time.September, 1)},
		{"back over the month start", day(2026, time.August, 1), -1, day(2026, time.July, 31)},
		{"over the year end", day(2026, time.December, 31), 1, day(2027, time.January, 1)},
	}
	for _, c := range cases {
		got := datePicker{cursor: c.from, open: true}.move(c.by).cursor
		if !sameDay(got, c.want) {
			t.Errorf("%s: move(%d) = %v, want %v", c.name, c.by, got, c.want)
		}
	}
}

func TestDaysInMonth(t *testing.T) {
	cases := []struct {
		year int
		mon  time.Month
		want int
	}{
		{2026, time.January, 31},
		{2026, time.February, 28},
		{2024, time.February, 29},
		{2000, time.February, 29},
		{1900, time.February, 28},
		{2026, time.April, 30},
	}
	for _, c := range cases {
		if got := daysInMonth(c.year, c.mon); got != c.want {
			t.Errorf("daysInMonth(%d, %v) = %d, want %d", c.year, c.mon, got, c.want)
		}
	}
}

func TestMonthGridPlacesDaysMondayFirst(t *testing.T) {
	// 01/08/2026 is a Saturday, so the first row is blank until column 5.
	g := monthGrid(day(2026, time.August, 9))

	if !g[0][5].Equal(day(2026, time.August, 1)) {
		t.Errorf("g[0][5] = %v, want 01/08/2026 (Saturday)", g[0][5])
	}
	if !g[0][6].Equal(day(2026, time.August, 2)) {
		t.Errorf("g[0][6] = %v, want 02/08/2026 (Sunday)", g[0][6])
	}
	for col := 0; col < 5; col++ {
		if !g[0][col].IsZero() {
			t.Errorf("g[0][%d] = %v, want a blank cell before the 1st", col, g[0][col])
		}
	}
	if !g[1][0].Equal(day(2026, time.August, 3)) {
		t.Errorf("g[1][0] = %v, want 03/08/2026 (the first Monday)", g[1][0])
	}
}

// Membership and uniqueness only; position is pinned by the tests around it.
func TestMonthGridCoversEveryDayWithoutDuplicates(t *testing.T) {
	g := monthGrid(day(2026, time.August, 9))
	seen := map[int]int{}
	for _, row := range g {
		for _, cell := range row {
			if cell.IsZero() {
				continue
			}
			if cell.Month() != time.August {
				t.Errorf("grid contains %v, from outside the month", cell)
			}
			seen[cell.Day()]++
		}
	}
	if len(seen) != 31 {
		t.Errorf("grid holds %d distinct days, want 31", len(seen))
	}
	for d, n := range seen {
		if n != 1 {
			t.Errorf("day %d appears %d times, want once", d, n)
		}
	}
}

func TestMonthGridFitsAMonthStartingSunday(t *testing.T) {
	// 01/02/2026 is a Sunday: the largest lead, so the 1st sits in column 6.
	g := monthGrid(day(2026, time.February, 1))
	if !g[0][6].Equal(day(2026, time.February, 1)) {
		t.Errorf("g[0][6] = %v, want 01/02/2026", g[0][6])
	}
	// Lead 6 + 28 days ends at index 33: row 4, column 5 (a Saturday).
	if !g[4][5].Equal(day(2026, time.February, 28)) {
		t.Errorf("g[4][5] = %v, want 28/02/2026 in the last populated cell", g[4][5])
	}
	if !g[4][6].IsZero() {
		t.Errorf("g[4][6] = %v, want blank — February 2026 ends on the Saturday", g[4][6])
	}
}

func TestMoveAcrossTheSantiagoDSTTransitionLandsOnTheNextDay(t *testing.T) {
	loc, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Skip("tzdata for America/Santiago not available:", err)
	}
	// Chile's clocks jump forward at midnight here; midnight-based AddDate
	// can land on the 13th at 23:00 instead of the 14th.
	seed := time.Date(2016, time.August, 13, 0, 0, 0, 0, loc)
	p := newDatePicker(seed).move(1)
	if y, m, d := p.cursor.Date(); !(y == 2016 && m == time.August && d == 14) {
		t.Errorf("cursor = %v, want 14/08/2016", p.cursor)
	}
}

func TestMonthGridHandlesTheSixRowWorstCase(t *testing.T) {
	// Lead 6 + 31 days ends at index 36, the sixth row.
	g := monthGrid(day(2026, time.March, 1)) // 01/03/2026 is a Sunday
	if !g[0][6].Equal(day(2026, time.March, 1)) {
		t.Errorf("g[0][6] = %v, want 01/03/2026", g[0][6])
	}
	if !g[5][1].Equal(day(2026, time.March, 31)) {
		t.Errorf("g[5][1] = %v, want 31/03/2026 in the sixth row", g[5][1])
	}
}

func TestPickerViewShowsMonthYearAndWeekdayHeader(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 9)))

	if !strings.Contains(out, "August 2026") {
		t.Errorf("View missing the month heading:\n%s", out)
	}
	if !strings.Contains(out, "Mo  Tu  We  Th  Fr  Sa  Su") {
		t.Errorf("View missing the Monday-first weekday header:\n%s", out)
	}
}

func TestPickerViewMarksTheCursor(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 1)))
	if !strings.Contains(out, "[ 9]") {
		t.Errorf("View does not bracket the selected day:\n%s", out)
	}
}

func TestPickerViewHasOneCursorOnly(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 1)))
	if got := strings.Count(out, "["); got != 1 {
		t.Errorf("View brackets %d days, want exactly 1:\n%s", got, out)
	}
}

func TestPickerViewRowsAllHaveTheSameWidth(t *testing.T) {
	// The cursor brackets must not widen its row; mid-week makes any extra
	// column show.
	out := newDatePicker(day(2026, time.August, 12)).View(day(2026, time.August, 1))
	lines := strings.Split(out, "\n")

	header := lipgloss.Width(lines[1]) // the Mo Tu We … row
	for i, line := range lines[2:] {
		if w := lipgloss.Width(line); w != header && strings.TrimSpace(stripANSI(line)) != "" {
			t.Errorf("week row %d is %d columns, want %d to match the header: %q",
				i, w, header, stripANSI(line))
		}
	}
}

func TestPickerViewShowsEveryDayOfTheMonth(t *testing.T) {
	out := stripANSI(newDatePicker(day(2026, time.August, 9)).View(day(2026, time.August, 9)))
	for _, d := range []string{"1", "15", "31"} {
		if !strings.Contains(out, d) {
			t.Errorf("View missing day %s:\n%s", d, out)
		}
	}
	if strings.Contains(out, "32") {
		t.Errorf("View shows a day past the end of the month:\n%s", out)
	}
}

func TestPickerViewFitsItsDeclaredWidth(t *testing.T) {
	// February 2026 starts on a Sunday — the widest leading-blank case.
	out := newDatePicker(day(2026, time.February, 1)).View(day(2026, time.February, 1))
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > pickerWidth {
			t.Errorf("line is %d columns, want at most %d: %q", w, pickerWidth, stripANSI(line))
		}
	}
}

func TestPickerViewRendersEveryMonthWithoutPanicking(t *testing.T) {
	for m := time.January; m <= time.December; m++ {
		for _, y := range []int{2024, 2026} { // one leap year, one not
			out := newDatePicker(day(y, m, 1)).View(day(2026, time.July, 30))
			if out == "" {
				t.Errorf("%v %d rendered empty", m, y)
			}
		}
	}
}

func TestPickerViewTodayAndCursorOnTheSameCell(t *testing.T) {
	d := day(2026, time.August, 9)
	out := stripANSI(newDatePicker(d).View(d))
	if !strings.Contains(out, "[ 9]") {
		t.Errorf("View does not bracket the day when it is both today and selected:\n%s", out)
	}
	if got := strings.Count(out, "["); got != 1 {
		t.Errorf("View brackets %d days, want exactly 1", got)
	}
	for _, line := range strings.Split(newDatePicker(d).View(d), "\n")[1:] {
		if w := lipgloss.Width(line); w != pickerWidth {
			t.Errorf("row is %d columns, want %d: %q", w, pickerWidth, stripANSI(line))
		}
	}
}

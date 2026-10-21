package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"gotodo/internal/task"
)

// Every page and popup stays inside the terminal at the supported sizes, so
// the renderer never cuts off the tab bar or a footer.
func TestEveryScreenFitsTerminal(t *testing.T) {
	withColor(t)
	for _, sz := range []struct {
		w, h int
		cols []task.Status
	}{{80, 24, nil}, {100, 30, task.DevColumns}} {
		b := &task.Board{Columns: sz.cols}
		d := ref.AddDate(0, 0, 2)
		p := b.Add("Ship the release with a title long enough to wrap", "notes\n\n- one\n- two", &d, ref)
		b.SetWorktree(p.ID, "../app-release", ref)
		b.AddChild(p.ID, "Write changelog", "", &d, ref)
		c := b.Add("Fix login", "", nil, ref)
		b.Move(c.ID, b.Statuses()[1], ref)

		var m tea.Model = NewApp(b)
		m, _ = m.Update(tea.WindowSizeMsg{Width: sz.w, Height: sz.h})
		check := func(name string) {
			out := m.View()
			if h := lipgloss.Height(out); h > sz.h {
				t.Errorf("%dx%d %s: %d rows tall:\n%s", sz.w, sz.h, name, h, stripANSI(out))
			}
			for i, line := range strings.Split(out, "\n") {
				if w := lipgloss.Width(line); w > sz.w {
					t.Errorf("%dx%d %s: line %d is %d wide", sz.w, sz.h, name, i, w)
					break
				}
			}
		}
		key := func(k tea.KeyMsg) { m, _ = m.Update(k) }
		runes := func(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
		esc := tea.KeyMsg{Type: tea.KeyEsc}

		check("board")
		key(runes("?"))
		check("help")
		key(runes("x"))
		key(tea.KeyMsg{Type: tea.KeyEnter})
		check("detail")
		key(esc)
		key(runes("e"))
		check("form")
		key(esc)
		key(runes("d"))
		check("confirm")
		key(runes("n"))
		key(runes("m"))
		check("move")
		key(esc)
		key(runes("f"))
		check("family")
		key(runes("f"))
		for _, page := range []string{"analytics", "archive", "calendar"} {
			key(tea.KeyMsg{Type: tea.KeyTab})
			check(page)
		}
	}
}

// hintBar breaks between items only and never exceeds its width.
func TestHintBarWrapsBetweenItems(t *testing.T) {
	items := []string{keyHint("a", "Add"), keyHint("e", "Edit"), keyHint("d", "Delete")}
	got := stripANSI(hintBar(12, items...))
	if got != "a Add  e Edit\nd Delete" && got != "a Add\ne Edit\nd Delete" {
		t.Errorf("hintBar(12) = %q", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if lipgloss.Width(line) > 12 {
			t.Errorf("line %q wider than 12", line)
		}
	}
}

// pageFrame pins the footer to the bottom row budget and clips a tall body.
func TestPageFramePinsFooterAndClips(t *testing.T) {
	out := stripANSI(pageFrame(40, 10, strings.Repeat("row\n", 20)+"last", keyHint("q", "Quit")))
	lines := strings.Split(out, "\n")
	if len(lines) != 9 { // terminal minus the tab bar
		t.Fatalf("pageFrame is %d rows, want 9:\n%s", len(lines), out)
	}
	if lines[8] != "q Quit" || lines[7] != "" {
		t.Errorf("footer not pinned under a blank row:\n%s", out)
	}
}

// withColor renders with ANSI styling for one test. Plain rendering trims
// styled trailing padding, which hides rows that wrap in a real terminal.
func withColor(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// Each form input fits its column in a real terminal, so no field wraps onto
// a ghost row and the sections stay one blank row apart.
func TestFormInputsDoNotWrap(t *testing.T) {
	withColor(t)
	m := fixedClock(NewBoardModel(&task.Board{}))
	m.SetSize(120, 40)
	m = press(m, "a")
	inner := m.popupWidth() - 2
	dl, wt := formDateWidths(inner)
	for _, c := range []struct {
		name  string
		view  string
		width int
	}{{"title", m.title.View(), inner}, {"parent", m.parent.View(), inner}, {"deadline", m.deadline.View(), dl}, {"worktree", m.worktree.View(), wt}} {
		if got := lipgloss.Width(c.view); got > c.width {
			t.Errorf("%s input is %d wide, column is %d", c.name, got, c.width)
		}
	}
	lines := strings.Split(stripANSI(m.renderForm()), "\n")
	for i, line := range lines {
		if strings.Contains(line, "Title") {
			if strings.Trim(lines[i+2], "│ ") != "" || !strings.Contains(lines[i+3], "Description") {
				t.Errorf("want Title, input, one blank row, Description:\n%s", strings.Join(lines, "\n"))
			}
		}
	}
}

// Countdown reads the gap in days, says "today" and "late", and stays empty
// for finished or undated tasks.
func TestCountdown(t *testing.T) {
	day := func(n int) *time.Time { d := ref.AddDate(0, 0, n); return &d }
	for _, c := range []struct {
		due    *time.Time
		status task.Status
		want   string
	}{
		{day(5), task.StatusTodo, "5d"},
		{day(1), task.StatusTodo, "1d"},
		{day(0), task.StatusTodo, "today"},
		{day(-3), task.StatusTodo, "late 3d"},
		{day(-3), task.StatusDone, ""},
		{nil, task.StatusTodo, ""},
	} {
		got := stripANSI(Countdown(task.Task{Deadline: c.due, Status: c.status}, ref, task.StatusDone))
		if got != c.want {
			t.Errorf("Countdown(%v, %s) = %q, want %q", c.due, c.status, got, c.want)
		}
	}
}

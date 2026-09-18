package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// ArchiveModel is the third page: finished tasks that have aged off the
// board. It is view-only — nothing here mutates the board.
type ArchiveModel struct {
	board *task.Board
	now   func() time.Time

	sel    int
	width  int
	height int
}

// NewArchiveModel wires a board into the archive page.
func NewArchiveModel(b *task.Board) ArchiveModel {
	return ArchiveModel{board: b, now: time.Now}
}

// SetSize records the terminal size for layout.
func (m *ArchiveModel) SetSize(w, h int) { m.width, m.height = w, h }

// Update handles selection movement. The archive is read-only, so no key
// here changes any task.
func (m ArchiveModel) Update(msg tea.Msg) (ArchiveModel, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	n := len(m.board.ArchivedTasks())
	switch k.String() {
	case "j", "down":
		m.sel++
	case "k", "up":
		m.sel--
	case "g":
		m.sel = 0
	case "G":
		m.sel = n - 1
	}
	if m.sel >= n {
		m.sel = n - 1
	}
	if m.sel < 0 {
		m.sel = 0
	}
	return m, nil
}

// archiveChrome is how many of the page's rows are spent on the heading and
// its blank line plus the blank line and footer at the bottom — the fixed
// cost around the entry list that fitWindow's avail must exclude.
const archiveChrome = 4

// View renders the archive list. Like the board's columns, the list of
// entries can be taller than the terminal, so it goes through the same
// fitWindow used by BoardModel to pick a scrolling window that keeps the
// selection visible and never grows past the terminal height.
func (m ArchiveModel) View() string {
	items := m.board.ArchivedTasks()
	width := m.width
	if width <= 0 {
		width = 80
	}
	height := m.height
	if height <= 0 {
		height = 24
	}

	head := TitleStyle.Render("ARCHIVE") +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")

	if len(items) == 0 {
		lines := []string{head, "",
			MutedStyle.Render("nothing archived yet"),
			"",
			MutedStyle.Render(truncate(fmt.Sprintf(
				"finished tasks move here %d days after you complete them",
				int(task.ArchiveAfter.Hours()/24)), width)),
		}
		return strings.Join(lines, "\n")
	}

	rendered := make([]string, len(items))
	heights := make([]int, len(items))
	for i, t := range items {
		rendered[i] = m.renderEntry(i, t, width)
		heights[i] = strings.Count(rendered[i], "\n") + 1
	}

	avail := height - archiveChrome
	start, end, more := fitWindow(heights, avail, m.sel)

	lines := []string{head, ""}
	for i := start; i < end; i++ {
		if i > start {
			lines = append(lines, "")
		}
		lines = append(lines, rendered[i])
	}
	if more > 0 {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("+%d more", more)))
	}
	lines = append(lines, "",
		MutedStyle.Render(truncate("j/k move · tab switch · read-only", width)))
	return strings.Join(lines, "\n")
}

func (m ArchiveModel) renderEntry(i int, t task.Task, width int) string {
	inner := width - 6

	rows := []string{truncate(t.Title, inner)}
	if t.Description != "" {
		rows = append(rows, descLine(t.Description, inner))
	}
	rows = append(rows, m.metaLine(t, inner))

	body := strings.Join(rows, "\n")
	if i == m.sel {
		return lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(ColAccent).
			PaddingLeft(1).Bold(true).Render(body)
	}
	return CardStyle.Render(body)
}

// archivedOn falls back to UpdatedAt for entries archived by an older build
// that did not stamp ArchivedAt.
func archivedOn(t task.Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}

// metaLine renders the entry's bottom line — the deadline plus when it was
// archived — and fits it inside inner columns rather than letting it
// overflow. This page has no minimum-width guard, so at narrow widths it
// drops the "· archived …" half first (the deadline is the more useful of
// the two) and, if even the deadline alone does not fit, truncates that.
func (m ArchiveModel) metaLine(t task.Task, inner int) string {
	archivedText := "archived " + FormatDate(archivedOn(t))
	dl := RenderDeadline(t, m.now(), m.board.DoneStatus())
	if dl == "" {
		return MutedStyle.Render(truncate(archivedText, inner))
	}

	full := dl + MutedStyle.Render("  ·  "+archivedText)
	if lipgloss.Width(full) <= inner {
		return full
	}
	if lipgloss.Width(dl) <= inner {
		return dl
	}

	// Even the deadline alone doesn't fit at this width. Truncate its plain
	// text and re-style, rather than slicing the ANSI-wrapped dl string.
	u := task.DeadlineUrgency(t, m.now(), m.board.DoneStatus())
	plain := "● " + FormatDate(*t.Deadline)
	if u == task.UrgencyOverdue {
		plain += " ✗"
	}
	return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render(truncate(plain, inner))
}

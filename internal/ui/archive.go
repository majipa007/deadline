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

// ArchiveModel is the read-only page of finished tasks aged off the board.
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

// Update moves the selection; no key here changes a task.
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

// archiveChrome is the rows around the entry list: tab bar, heading and its
// blank line, pageFrame's blank line and footer.
const archiveChrome = 5

// View renders the archive list, windowed by fitWindow to keep the
// selection visible within the terminal height.
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

	head := TitleStyle.Render("Archive") +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")

	hints := append([]string{MutedStyle.Render("read-only"), keyHint("j/k", "Select")}, globalHints()...)
	if len(items) == 0 {
		lines := []string{head, "",
			MutedStyle.Render("nothing archived yet"),
			"",
			MutedStyle.Render(truncate(fmt.Sprintf(
				"finished tasks move here %d days after you complete them",
				int(task.ArchiveAfter.Hours()/24)), width)),
		}
		return pageFrame(m.width, m.height, strings.Join(lines, "\n"), hints...)
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
	return pageFrame(m.width, m.height, strings.Join(lines, "\n"), hints...)
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
		return CardSelectedStyle.Render(body)
	}
	return CardStyle.Render(body)
}

// archivedOn falls back to UpdatedAt for entries written without ArchivedAt.
func archivedOn(t task.Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}

// metaLine renders deadline and archive date within inner columns; when
// narrow it drops the archive date first, then truncates the deadline.
func (m ArchiveModel) metaLine(t task.Task, inner int) string {
	archivedText := "archived " + FormatDate(archivedOn(t))
	dl := RenderDeadline(t, m.now(), m.board.DoneStatus())
	if dl == "" {
		return MutedStyle.Render(truncate(archivedText, inner))
	}

	full := dl + MutedStyle.Render("   "+archivedText)
	if lipgloss.Width(full) <= inner {
		return full
	}
	if lipgloss.Width(dl) <= inner {
		return dl
	}

	// Truncate plain text and re-style; slicing the styled dl would cut ANSI codes.
	u := task.DeadlineUrgency(t, m.now(), m.board.DoneStatus())
	plain := "● " + FormatDate(*t.Deadline)
	if u == task.UrgencyOverdue {
		plain += " ✗"
	}
	return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render(truncate(plain, inner))
}

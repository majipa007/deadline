package ui

import (
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type page int

const (
	pageBoard page = iota
	pageAnalytics
	pageArchive
	pageCalendar
	pageCount
)

// AppModel is the root Bubble Tea model: it owns page switching, the help
// overlay, and persistence.
type AppModel struct {
	board     BoardModel
	analytics AnalyticsModel
	archive   ArchiveModel
	calendar  CalendarModel
	store     *task.Board

	page     page
	showHelp bool
	saveErr  string

	width  int
	height int

	// boardMod/boardSize track the board file as last seen, so the refresh
	// tick can spot writes from other sessions (agents logging headless).
	boardMod  time.Time
	boardSize int64

	now func() time.Time
}

// NewApp builds the root model around a loaded board.
func NewApp(b *task.Board) AppModel {
	return AppModel{
		board:     NewBoardModel(b),
		analytics: NewAnalyticsModel(b),
		archive:   NewArchiveModel(b),
		calendar:  NewCalendarModel(b),
		store:     b,
		now:       time.Now,
	}
}

// archiveTickMsg fires periodically so a long-running session still tidies
// itself instead of only archiving at startup.
type archiveTickMsg time.Time

func archiveTick() tea.Cmd {
	return tea.Tick(time.Hour, func(t time.Time) tea.Msg { return archiveTickMsg(t) })
}

// refreshInterval is how often an open board polls for writes from other
// sessions; it costs one stat call when nothing changed.
const refreshInterval = 2 * time.Second

// refreshTickMsg fires every refreshInterval to pick up headless writes.
type refreshTickMsg time.Time

func refreshTick() tea.Cmd {
	return tea.Tick(refreshInterval, func(t time.Time) tea.Msg { return refreshTickMsg(t) })
}

// Init starts the hourly archive sweep and the refresh poll.
func (m AppModel) Init() tea.Cmd { return tea.Batch(archiveTick(), refreshTick()) }

// Update routes global keys itself and forwards the rest to the active page.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.board.SetSize(msg.Width, msg.Height)
		m.analytics.SetSize(msg.Width, msg.Height)
		m.archive.SetSize(msg.Width, msg.Height)
		m.calendar.SetSize(msg.Width, msg.Height)
		return m, nil

	case dirtyMsg:
		if err := m.store.Save(); err != nil {
			m.saveErr = err.Error()
		} else {
			m.saveErr = ""
		}
		m.noteBoardStat()
		return m, nil

	case archiveTickMsg:
		selected := ""
		if t, ok := m.board.selectedTask(); ok {
			selected = t.ID
		}
		if m.store.SweepArchive(m.now()) > 0 {
			m.board.reconcileFamily(selected)
			return m, tea.Batch(dirty(), archiveTick())
		}
		return m, archiveTick()

	case refreshTickMsg:
		// Re-arm first: every path below must keep polling.
		cmd := refreshTick()
		// Only reload a clean, non-modal board so nothing mid-edit is lost;
		// the next tick retries otherwise.
		if m.board.mode == modeNormal && !m.store.Dirty() {
			if changed, _ := m.boardChanged(); changed {
				selected := ""
				if t, ok := m.board.selectedTask(); ok {
					selected = t.ID
				}
				// Clean, so a wholesale reload loses nothing; keep tombstones.
				if fresh, err := task.Load(m.store.Path()); err == nil {
					fresh.CarryTombstonesFrom(m.store)
					*m.store = *fresh
					m.board.reconcileFamily(selected)
					m.noteBoardStat()
				}
			}
		}
		return m, cmd

	case tea.KeyMsg:
		// Non-normal board modes own their keys; only ctrl+c is always global.
		typing := m.page == pageBoard && m.board.mode != modeNormal
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Bubbletea coalesces fast input (held keys, paste) into one
		// KeyRunes message; replay it one rune at a time.
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			var cmds []tea.Cmd
			var next tea.Model = m
			for _, r := range msg.Runes {
				helpWasOpen := next.(AppModel).showHelp
				var cmd tea.Cmd
				next, cmd = next.(AppModel).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: msg.Alt})
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
				if helpWasOpen {
					break // the overlay swallowed this batch
				}
			}
			return next, tea.Batch(cmds...)
		}
		if !typing {
			if m.showHelp {
				m.showHelp = false // any key closes the overlay, and does nothing else
				return m, nil
			}
			switch msg.String() {
			case "tab":
				m.page = (m.page + 1) % pageCount
				return m, nil
			case "?":
				m.showHelp = !m.showHelp
				return m, nil
			case "q":
				return m, tea.Quit
			}
		}
		switch m.page {
		case pageBoard:
			var cmd tea.Cmd
			m.board, cmd = m.board.Update(msg)
			return m, cmd
		case pageArchive:
			var cmd tea.Cmd
			m.archive, cmd = m.archive.Update(msg)
			return m, cmd
		case pageCalendar:
			var cmd tea.Cmd
			m.calendar, cmd = m.calendar.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	return m, nil
}

// View renders the tab bar above the active page or the help overlay.
func (m AppModel) View() string {
	var body string
	switch {
	case m.showHelp:
		body = m.renderHelp()
	case m.page == pageAnalytics:
		body = m.analytics.View()
	case m.page == pageArchive:
		body = m.archive.View()
	case m.page == pageCalendar:
		body = m.calendar.View()
	default:
		body = m.board.View()
	}
	// A save error rides on the tab bar row, so it costs no page any height.
	tabs := m.renderTabs()
	if m.saveErr != "" {
		tabs += "  " + errorLine(truncate("save failed: "+m.saveErr, max(m.width-lipgloss.Width(tabs)-4, 10)))
	}
	return lipgloss.JoinVertical(lipgloss.Left, tabs, body)
}

// noteBoardStat records the board file's current mtime and size.
func (m *AppModel) noteBoardStat() {
	if st, err := os.Stat(m.store.Path()); err == nil {
		m.boardMod, m.boardSize = st.ModTime(), st.Size()
	}
}

// boardChanged reports whether the board file differs from the last noted
// state. A vanished file is not a change: the next save recreates it.
func (m AppModel) boardChanged() (bool, error) {
	st, err := os.Stat(m.store.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return !st.ModTime().Equal(m.boardMod) || st.Size() != m.boardSize, nil
}

func (m AppModel) renderTabs() string {
	// Active tab is a filled accent chip.
	active := lipgloss.NewStyle().Bold(true).Padding(0, 2).
		Foreground(lipgloss.AdaptiveColor{Light: "#ffffff", Dark: "#1a1b26"}).Background(ColAccent)
	inactive := MutedStyle.Copy().Padding(0, 2)

	names := [pageCount]string{"Board", "Analytics", "Archive", "Calendar"}
	tabs := make([]string, 0, len(names))
	for i, n := range names {
		if page(i) == m.page {
			tabs = append(tabs, active.Render(n))
			continue
		}
		tabs = append(tabs, inactive.Render(n))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// helpSection renders one titled group of help rows.
func helpSection(title string, rows [][2]string) string {
	key := lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Width(9)
	lines := []string{TitleStyle.Render(title)}
	for _, r := range rows {
		lines = append(lines, key.Render(r[0])+MutedStyle.Render(r[1]))
	}
	return strings.Join(lines, "\n")
}

// renderHelp is the key reference panel: two columns when wide, stacked when narrow.
func (m AppModel) renderHelp() string {
	left := helpSection("Navigation", [][2]string{
		{"tab", "next page"},
		{"h l", "previous / next column"},
		{"j k", "previous / next task"},
		{"g G", "first / last in column"},
		{"ctrl+t", "column / item focus"},
		{"enter", "open the selected task"},
	}) + "\n\n" + helpSection("Tasks", [][2]string{
		{"a", "add a task"},
		{"s", "add a subtask"},
		{"e", "edit the selected task"},
		{"d", "delete (asks first)"},
		{"m", "grab, h/l move, enter drop"},
		{"f", "show only this family"},
	})
	urgency := func(u task.Urgency, s string) string {
		return lipgloss.NewStyle().Foreground(UrgencyColor(u)).Render("● " + s)
	}
	right := helpSection("Form", [][2]string{
		{"tab", "next field"},
		{"ctrl+j", "new line in description"},
		{"deadline", "hjkl pick a day, t today"},
		{"worktree", "path or branch, optional"},
		{"parent", "type to filter, ↑/↓ enter"},
		{"enter", "save, esc cancel"},
	}) + "\n\n" + helpSection("Pages", [][2]string{
		{"archive", "j/k select"},
		{"calendar", "h/l month, t today"},
		{"? q", "this help, quit"},
	}) + "\n\n" + helpSection("Deadlines", [][2]string{
		{urgency(task.UrgencyFuture, "green"), "more than 3 days left"},
		{urgency(task.UrgencySoon, "amber"), "3 days or fewer"},
		{urgency(task.UrgencyUrgent, "red"), "1 day or fewer, or late"},
	})

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
	if lipgloss.Width(body)+4 > m.width && m.width > 0 {
		body = left + "\n\n" + right
	}
	w := lipgloss.Width(body)
	rows := append([]string{TitleStyle.Render("Keys"), MutedStyle.Render(strings.Repeat("─", w)), body},
		popupFoot(w, keyHint("any key", "Close"))...)
	panel := ColumnStyle.Render(strings.Join(rows, "\n"))
	return m.board.overlay(panel)
}

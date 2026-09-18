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

// refreshInterval is how often an open board looks for writes from other
// sessions. Two seconds keeps an agent's cards appearing live without any
// measurable cost: it is one stat call when nothing changed.
const refreshInterval = 2 * time.Second

// refreshTickMsg fires on that interval so an open board picks up headless
// writes (agents logging in parallel) without any keypress.
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
		if m.store.SweepArchive(m.now()) > 0 {
			m.board.clampSelection()
			return m, tea.Batch(dirty(), archiveTick())
		}
		return m, archiveTick()

	case refreshTickMsg:
		// Re-arm first: every path below must keep polling.
		cmd := refreshTick()
		// Only refresh over a clean, non-modal board. Unsaved keystrokes
		// (dirty) or an open form/move/confirm/detail must never be
		// yanked away mid-interaction; the next tick retries instead.
		if m.board.mode == modeNormal && !m.store.Dirty() {
			if changed, _ := m.boardChanged(); changed {
				// Clean means memory holds nothing the disk lacks, so a
				// wholesale reload drops nothing of ours. Tombstones are
				// carried over so a concurrently re-saved deleted task is
				// not merged back on the next Save.
				if fresh, err := task.Load(m.store.Path()); err == nil {
					fresh.CarryTombstonesFrom(m.store)
					*m.store = *fresh
					m.board.clampSelection()
					m.noteBoardStat()
				}
			}
		}
		return m, cmd

	case tea.KeyMsg:
		// Every non-normal board mode is modal with respect to the global
		// keys: modeInput is text entry, modeConfirm and modeMove each
		// advertise their own q/tab-shaped footer (e.g. "y / n",
		// "enter drop · esc cancel") that would otherwise be preempted.
		// Only ctrl+c is global regardless of mode.
		typing := m.page == pageBoard && m.board.mode != modeNormal
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		// Bubbletea coalesces bytes from a single read into one KeyRunes
		// message when keys arrive faster than we consume them (held-down
		// navigation keys, fast typing, paste). Split multi-rune messages
		// into one KeyMsg per rune so every keypress is still handled.
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

// View renders the tab bar above the active page, or the help overlay.
// The bar names the pages; the way out is a footer hint on each page
// ("tab switch"), next to the keys that live there.
func (m AppModel) View() string {
	if m.showHelp {
		return m.renderHelp()
	}
	body := m.board.View()
	switch m.page {
	case pageAnalytics:
		body = m.analytics.View()
	case pageArchive:
		body = m.archive.View()
	case pageCalendar:
		body = m.calendar.View()
	}
	parts := []string{m.renderTabs(), body}
	if m.saveErr != "" {
		parts = append(parts, HelpStyle.Render("save failed: "+m.saveErr))
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// noteBoardStat records the board file as currently on disk. Called after
// every save and refresh so the next tick compares against fresh state.
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
	active := lipgloss.NewStyle().Bold(true).Foreground(ColAccent).Padding(0, 2)
	inactive := MutedStyle.Copy().Padding(0, 2)

	names := [pageCount]string{"BOARD", "ANALYTICS", "ARCHIVE", "CALENDAR"}
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

func (m AppModel) renderHelp() string {
	rows := [][2]string{
		{"tab", "cycle board → analytics → archive → calendar"},
		{"ctrl+t", "toggle column / item focus"},
		{"h l", "previous / next column"},
		{"j k", "previous / next task (item focus)"},
		{"g G", "first / last task in column"},
		{"enter", "expand the selected task in a popup (e edit, d delete, esc close)"},
		{"a", "add a task"},
		{"e", "edit the selected task"},
		{"d", "delete the selected task (confirms)"},
		{"ctrl+j", "new line, while writing a description"},
		{"deadline", "a calendar opens with the field; hjkl to move, t for today"},
		{"m", "grab the task, then h/l to move, enter to drop, esc to cancel"},
		{"?", "toggle this help"},
		{"q", "quit"},
		{"", ""},
		{"calendar", "every deadline on a month grid; h/l changes month, t back to today"},
		{"deadlines", "green >3 days · amber ≤3 · red ≤1 · red ✗ overdue"},
	}
	lines := []string{TitleStyle.Render("KEYS"), ""}
	for _, r := range rows {
		lines = append(lines,
			lipgloss.NewStyle().Foreground(ColAccent).Width(10).Render(r[0])+
				MutedStyle.Render(r[1]))
	}
	lines = append(lines, "", MutedStyle.Render("any key to close"))
	return ColumnStyle.Render(strings.Join(lines, "\n"))
}

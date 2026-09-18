package ui

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

type focusMode int

const (
	focusItem focusMode = iota
	focusColumn
)

type boardMode int

const (
	modeNormal boardMode = iota
	modeInput
	modeMove
	modeConfirm
	modeDetail
)

// Fields of the add/edit form, in tab order.
const (
	fieldTitle = iota
	fieldDesc
	fieldDeadline
	fieldCount
)

// BoardModel is the kanban page.
type BoardModel struct {
	board *task.Board

	col   int   // focused column, index into the board's columns
	sel   []int // selected item per column, same length as the board's columns
	focus focusMode
	mode  boardMode

	title    textinput.Model
	desc     textarea.Model // multi-line: ctrl+j inserts a newline, enter saves
	deadline textinput.Model
	field    int        // which of the three fields has focus
	picker   datePicker // the deadline calendar, when open

	editID   string      // set while editing an existing task
	grabID   string      // set while in modeMove
	grabFrom task.Status // original column of the grabbed task

	now func() time.Time // injectable clock; tests pin it

	width      int
	height     int
	err        string
	footerRows int // height of the last-rendered footer; set by View, read by columnHeight
}

// NewBoardModel wires a board into a fresh page model.
func NewBoardModel(b *task.Board) BoardModel {
	m := BoardModel{board: b, focus: focusItem, mode: modeNormal, now: time.Now}
	m.sel = make([]int, len(b.Statuses()))

	line := func(placeholder string, limit int) textinput.Model {
		in := textinput.New()
		in.Placeholder = placeholder
		in.CharLimit = limit
		in.Prompt = "› "
		return in
	}
	m.title = line("task title", 200)
	m.deadline = line("DD/MM/YYYY (optional)", 10)

	m.desc = textarea.New()
	m.desc.Placeholder = "description (optional) · ctrl+j for a new line"
	m.desc.CharLimit = 500
	m.desc.Prompt = "› "
	m.desc.ShowLineNumbers = false
	// Enter must reach updateInput to save the task, so the newline moves to
	// ctrl+j. Terminals send 0x0A for ctrl+j and 0x0D for enter, so the two
	// stay distinguishable.
	m.desc.KeyMap.InsertNewline.SetKeys("ctrl+j")
	m.sizeForm()
	return m
}

// SetSize records the terminal size for layout.
func (m *BoardModel) SetSize(w, h int) {
	m.width, m.height = w, h
	m.sizeForm()
}

const (
	// popupMaxWidth caps the centred detail/form panel: past this a line of
	// description is too long to read comfortably.
	popupMaxWidth = 64
	// labelWidth is the form's left gutter, wide enough for "Description".
	labelWidth = 12
	// descRows is how many rows of the description are visible at once; the
	// textarea scrolls past that.
	descRows = 4
)

// popupWidth is the content width of the centred panels, leaving a margin so
// the popup reads as floating over the board rather than filling the screen.
func (m BoardModel) popupWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	w -= 8
	if w > popupMaxWidth {
		w = popupMaxWidth
	}
	if w < minColumnWidth {
		w = minColumnWidth
	}
	return w
}

// sizeForm fits the three fields inside the popup, so a long value scrolls
// inside its own box instead of stretching the panel past the terminal.
func (m *BoardModel) sizeForm() {
	inner := m.popupWidth() - labelWidth - 2 // 2 for the panel's own padding
	if inner < 10 {
		inner = 10
	}
	m.title.Width = inner - 2 // textinput.Width excludes its "› " prompt
	m.deadline.Width = inner - 2
	m.desc.SetWidth(inner)
	rows, _ := m.formRows()
	m.desc.SetHeight(rows)
}

// formRows splits the terminal's height between the popup's fixed furniture,
// the calendar, and the description box: rows is how tall the description box
// may be, calendar whether the calendar still fits. Nothing else in the form
// can shrink, so on a terminal too short for both the calendar is what gives
// — the date can still be typed. Callers must re-run this (via sizeForm)
// whenever the height or the picker changes.
func (m BoardModel) formRows() (rows int, calendar bool) {
	rows, calendar = descRows, m.picker.open
	if m.height <= 0 {
		return rows, calendar
	}
	fixed := 8 // 2 page rows + 2 border + heading + title + deadline + hint
	if m.err != "" {
		fixed++
	}
	cal := 0
	if calendar {
		cal = lipgloss.Height(m.picker.View(m.now())) + 3 // 2 blank rows + its hint
	}
	if m.height-fixed-cal < 1 {
		calendar, cal = false, 0
	}
	if spare := m.height - fixed - cal; spare < rows {
		rows = spare
	}
	if rows < 1 {
		rows = 1
	}
	return rows, calendar
}

// overlay centres a panel on the page. lipgloss has no compositing at this
// version, so the popup replaces the board rather than floating over it.
// ponytail: no true overlay, revisit if the board behind is worth the
// ANSI-aware line surgery it would take.
func (m BoardModel) overlay(panel string) string {
	w, h := m.width, m.height-2 // AppModel joins the tab bar on above us
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 22
	}
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, panel)
}

func (m BoardModel) currentStatus() task.Status { return m.board.Statuses()[m.col] }

// selectedTask is the card under the cursor, if the column is not empty.
func (m BoardModel) selectedTask() (task.Task, bool) {
	items := m.board.ByStatus(m.currentStatus())
	i := m.sel[m.col]
	if i < 0 || i >= len(items) {
		return task.Task{}, false
	}
	return items[i], true
}

// clampSelection keeps every column's cursor inside its item range.
func (m *BoardModel) clampSelection() {
	for i, s := range m.board.Statuses() {
		n := len(m.board.ByStatus(s))
		if m.sel[i] >= n {
			m.sel[i] = n - 1
		}
		if m.sel[i] < 0 {
			m.sel[i] = 0
		}
	}
}

const (
	// minColumnWidth is the floor set by the widest deadline line
	// ("● DD/MM/YYYY ✗", 14 columns) plus the card's own border and padding.
	// Do not lower this without re-checking
	// TestCardDeadlineFitsAtMinimumColumnWidth.
	minColumnWidth = 18
	// columnChrome is how much wider than its content each rendered column
	// is: ColumnStyle's one-column border plus one-column padding, on each
	// side that columnWidth's "w/n - columnChrome" already subtracts for.
	columnChrome = 2
)

// columnWidth splits the terminal evenly across the board's columns, leaving
// room for each column's border and padding.
func (m BoardModel) columnWidth() int {
	w := m.width
	if w <= 0 {
		w = 80
	}
	per := w/len(m.board.Statuses()) - columnChrome
	if per < minColumnWidth {
		per = minColumnWidth
	}
	return per
}

// minBoardWidth is the narrowest terminal that can show every column at
// minColumnWidth without wrapping. View falls back to a warning below this.
func (m BoardModel) minBoardWidth() int {
	return len(m.board.Statuses()) * (minColumnWidth + columnChrome)
}

// minColumnBlockHeight is the least columnHeight() will ever return: header,
// blank line, one card row, plus the column's own top+bottom border.
const minColumnBlockHeight = 5

// rawColumnHeight is columnHeight() before its floor is applied — what the
// terminal actually has room for, which can go negative on a very short
// terminal. Reserves one row for the tab bar (AppModel joins that on above
// this model's own View), two for the column block's border, and one spare
// row so the board isn't rendered flush against the very last line.
func (m BoardModel) rawColumnHeight() int {
	return m.height - 4 - m.footerRows
}

func (m BoardModel) columnHeight() int {
	h := m.rawColumnHeight()
	if h < minColumnBlockHeight {
		h = minColumnBlockHeight
	}
	return h
}

// View renders the board's columns side by side plus the footer line, or a
// centred popup when a task is expanded or the form is open. Below
// minBoardWidth the columns would overflow and wrap into a scrambled mess,
// so it renders a short warning instead — the popups are exempt, they size
// themselves and stay readable on a narrow terminal. m.width == 0 (before
// the first WindowSizeMsg) falls through to the normal path, which defaults
// to 80.
//
// The footer is measured before the columns are laid out so columnHeight has
// its real height rather than a hard-coded guess. m has a value receiver, so
// the recorded height is stashed on this local copy before it is used below.
func (m BoardModel) View() string {
	switch m.mode {
	case modeDetail:
		return m.overlay(m.renderDetail())
	case modeInput:
		return m.overlay(m.renderForm())
	}
	if need := m.minBoardWidth(); m.width > 0 && m.width < need {
		return MutedStyle.Render(fmt.Sprintf(
			"terminal too narrow\n\ngotodo needs at least %d columns for the %d-column board.\nThis terminal is %d. Widen it, or press tab for Analytics or Archive.",
			need, len(m.board.Statuses()), m.width))
	}
	footer := m.renderFooter()
	m.footerRows = lipgloss.Height(footer)

	cw := m.columnWidth()
	cols := make([]string, 0, len(m.board.Statuses()))
	for i, s := range m.board.Statuses() {
		cols = append(cols, m.renderColumn(i, s, cw))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func (m BoardModel) renderColumn(idx int, s task.Status, width int) string {
	items := m.board.ByStatus(s)

	header := HeaderStyle(s).Render(s.Label()) +
		MutedStyle.Render(" ("+strconv.Itoa(len(items))+")")

	var lines []string
	lines = append(lines, header, "")

	if len(items) == 0 {
		lines = append(lines, MutedStyle.Render("empty"))
	}
	lines = append(lines, m.renderCardWindow(idx, items, width)...)

	style := ColumnStyle.Copy()
	if idx == m.col {
		style = ColumnFocusedStyle.Copy()
		if m.focus == focusColumn {
			style = style.BorderForeground(AccentFor(s))
		}
	}
	return style.Width(width).Height(m.columnHeight()).
		Render(strings.Join(lines, "\n"))
}

// renderCardWindow renders as many cards as fit within the column's height
// (lipgloss.Style.Height only pads, it does not clip), separated by a blank
// line. When some cards do not fit it emits a trailing "+N more" line, and
// scrolls the window down so the currently selected card is always shown
// rather than clipped away.
func (m BoardModel) renderCardWindow(idx int, items []task.Task, width int) []string {
	avail := m.columnHeight() - 2 // header + blank line already emitted

	sel := -1
	if idx == m.col {
		sel = m.sel[m.col]
	}

	rendered := make([]string, len(items))
	heights := make([]int, len(items))
	for i, t := range items {
		rendered[i] = m.renderCard(idx, i, t, width)
		heights[i] = strings.Count(rendered[i], "\n") + 1
	}

	start, end, more := fitWindow(heights, avail, sel)

	var lines []string
	for i := start; i < end; i++ {
		if i > start {
			lines = append(lines, "")
		}
		lines = append(lines, rendered[i])
	}
	if more > 0 {
		lines = append(lines, MutedStyle.Render(fmt.Sprintf("+%d more", more)))
	}
	return lines
}

// fitWindow picks the run of entries [start, end) that fits within avail
// rows — heights[i] gives entry i's own row count, and every entry after
// the first in the window costs one more row for its blank separator — while
// scrolling so the entry at sel (if any, sel < 0 means nothing is selected)
// stays inside the window rather than falling off past the fold. more
// reports how many trailing entries were left out, after reserving a row
// for the "+N more" line the caller appends when it is non-zero.
//
// Shared by BoardModel's card columns and ArchiveModel's single list: both
// need the same variable-height, keep-the-selection-visible windowing.
func fitWindow(heights []int, avail, sel int) (start, end, more int) {
	if avail < 1 {
		avail = 1
	}
	n := len(heights)

	// windowRows is the row count of [s,e), including the blank separator
	// before every entry after the first in the window.
	windowRows := func(s, e int) int {
		rows := 0
		for i := s; i < e; i++ {
			if i > s {
				rows++
			}
			rows += heights[i]
		}
		return rows
	}

	// fit grows e greedily from s while the window still fits in avail.
	fit := func(s int) int {
		e := s
		for e < n && windowRows(s, e+1) <= avail {
			e++
		}
		return e
	}

	start = 0
	end = fit(start)
	// Scroll down until the selected entry is inside the window.
	for sel >= end && start < n-1 {
		start++
		end = fit(start)
	}

	more = n - end
	if more > 0 {
		// Reserve one row for the "+N more" line.
		for end > start && windowRows(start, end)+1 > avail {
			end--
		}
		more = n - end
	}
	return start, end, more
}

// renderCard draws one card: title, optional description, optional deadline.
// A card is 1 to 3 lines tall depending on which fields are set.
func (m BoardModel) renderCard(colIdx, itemIdx int, t task.Task, width int) string {
	inner := width - 4
	selected := colIdx == m.col && itemIdx == m.sel[m.col]
	grabbed := selected && m.mode == modeMove && t.ID == m.grabID

	title := truncate(t.Title, inner)
	if grabbed {
		title = "⇄ " + truncate(t.Title, inner-2)
	}

	lines := []string{title}
	if t.Description != "" {
		lines = append(lines, descLine(t.Description, inner))
	}
	if dl := RenderDeadline(t, m.now(), m.board.DoneStatus()); dl != "" {
		lines = append(lines, dl)
	}
	body := strings.Join(lines, "\n")

	switch {
	case grabbed:
		return CardGrabbedStyle.Render(body)
	case selected && m.focus == focusItem:
		return CardSelectedStyle.Render(body)
	default:
		return CardStyle.Render(body)
	}
}

// descLine is the one-line description a list entry shows. A multi-line
// description collapses to its first line plus an ellipsis, so a long note
// cannot stretch a card past the row budget its column planned for it — the
// whole thing is one enter away in the detail popup. Shared by the board's
// cards and the archive's list.
func descLine(desc string, width int) string {
	first, rest, _ := strings.Cut(desc, "\n")
	if strings.TrimSpace(rest) != "" {
		first += " …"
	}
	return MutedStyle.Render(truncate(first, width))
}

// rowsHeight is how many terminal rows a slice of panel rows occupies —
// lipgloss.Height of the joined block, except that no rows occupy none
// (joining an empty slice would otherwise measure as one blank row).
func rowsHeight(rows []string) int {
	if len(rows) == 0 {
		return 0
	}
	return lipgloss.Height(strings.Join(rows, "\n"))
}

// detailBudget is how many content rows the detail panel may use before it
// overflows the terminal: the border costs two rows and View's own reserve
// another two. A terminal size we have not been told yet is unbounded.
func (m BoardModel) detailBudget() int {
	if m.height <= 0 {
		return 1 << 30
	}
	return m.height - 4
}

// renderDetail is the centred popup for the selected task: the full title and
// description wrapped rather than truncated, and the deadline shown on the
// same month calendar the form uses.
//
// There is no scrolling here, so on a terminal too short for everything the
// panel gives things up in order rather than overflowing: the calendar goes
// first, then the description is clipped to what is left with an ellipsis
// marking the cut. The title, status, deadline line and hint always stay.
// ponytail: clipping, not a viewport — add one if reading long descriptions
// on a short terminal becomes a real habit.
func (m BoardModel) renderDetail() string {
	t, ok := m.selectedTask()
	if !ok {
		return ""
	}
	w := m.popupWidth()
	wrap := lipgloss.NewStyle().Width(w)

	head := []string{
		wrap.Copy().Bold(true).Foreground(AccentFor(t.Status)).Render(t.Title),
		MutedStyle.Render(t.Status.Label()),
	}
	foot := []string{"", MutedStyle.Render("e edit · d delete · esc close")}

	var when []string
	if dl := RenderDeadline(t, m.now(), m.board.DoneStatus()); dl != "" {
		when = []string{"", dl}
		// The deadline's own month, the day bracketed and today green, so
		// "how far off is this" reads at a glance instead of from date
		// arithmetic. Anchored to now's zone, the zone RenderDeadline and
		// DeadlineUrgency both work in.
		cal := []string{"", newDatePicker(t.Deadline.In(m.now().Location())).View(m.now())}
		if rowsHeight(head)+rowsHeight(when)+rowsHeight(cal)+rowsHeight(foot) <= m.detailBudget() {
			when = append(when, cal...)
		}
	}

	var desc []string
	if t.Description != "" {
		desc = append([]string{""},
			strings.Split(wrap.Copy().Foreground(ColText).Render(t.Description), "\n")...)
		room := m.detailBudget() - rowsHeight(head) - rowsHeight(when) - rowsHeight(foot)
		switch {
		case room < 2: // not even a blank line and one line of text
			desc = nil
		case room < len(desc):
			desc = append(desc[:room-1], MutedStyle.Render("…"))
		}
	}

	rows := append(append(append(head, desc...), when...), foot...)
	return ColumnStyle.Copy().Width(w).Render(strings.Join(rows, "\n"))
}

// parseDeadline reads a DD/MM/YYYY date. Blank means "no deadline", which is
// not an error. The layout matches FormatDate, so what the card shows is
// exactly what you type back in.
//
// Parsed in the local zone, not UTC: DaysUntilDeadline re-anchors the
// deadline to now's location before taking its calendar day. Parsing into
// UTC would shift that calendar day back for anyone west of UTC, making a
// deadline typed for "today" read as overdue on the day it's due.
func parseDeadline(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	d, err := time.ParseInLocation("02/01/2006", s, time.Local)
	if err != nil {
		return nil, errors.New("deadline must look like 02/08/2026, or be left blank")
	}
	return &d, nil
}

// openForm puts the model into the add/edit form, seeded with the given
// values and focused on the title.
func (m *BoardModel) openForm(editID, title, desc string, deadline *time.Time) {
	m.mode = modeInput
	m.editID = editID
	m.field = fieldTitle
	m.err = ""

	dl := ""
	if deadline != nil {
		dl = FormatDate(*deadline)
	}
	m.title.SetValue(title)
	m.desc.SetValue(desc)
	m.deadline.SetValue(dl)
	m.applyFieldFocus() // sizes the fields too
}

// closeForm returns to normal mode and drops focus from every field.
func (m *BoardModel) closeForm() {
	m.mode = modeNormal
	m.editID = ""
	m.err = ""
	m.picker = datePicker{}
	m.title.Blur()
	m.desc.Blur()
	m.deadline.Blur()
}

// applyFieldFocus puts the cursor in m.field's widget and takes it off the
// other two, and opens the calendar only on the Deadline field.
func (m *BoardModel) applyFieldFocus() {
	m.title.Blur()
	m.desc.Blur()
	m.deadline.Blur()
	switch m.field {
	case fieldTitle:
		m.title.Focus()
		m.title.CursorEnd()
	case fieldDesc:
		m.desc.Focus()
	case fieldDeadline:
		m.deadline.Focus()
		m.deadline.CursorEnd()
	}
	if m.field == fieldDeadline {
		m.seedPicker()
	} else {
		m.picker = datePicker{}
	}
	m.sizeForm() // the calendar's rows come out of the description box
}

// focusField moves form focus by delta, wrapping at both ends.
func (m *BoardModel) focusField(delta int) {
	m.field = (m.field + delta + fieldCount) % fieldCount
	m.applyFieldFocus()
}

// renderForm draws the three-field entry panel, centred by View.
func (m BoardModel) renderForm() string {
	heading := "new task"
	if m.editID != "" {
		heading = "edit task"
	}
	label := func(i int, text string) string {
		style := MutedStyle
		if i == m.field {
			style = lipgloss.NewStyle().Foreground(ColAccent).Bold(true)
		}
		return style.Render(fmt.Sprintf("%-*s", labelWidth, text))
	}

	rows := []string{
		TitleStyle.Render(heading),
		label(fieldTitle, "Title") + m.title.View(),
		// The description box is several rows tall, so its label is joined
		// alongside rather than concatenated onto the first line.
		lipgloss.JoinHorizontal(lipgloss.Top, label(fieldDesc, "Description"), m.desc.View()),
		label(fieldDeadline, "Deadline") + m.deadline.View(),
	}
	if m.err != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(AccentFor(task.StatusBlocked)).
			Render("! "+m.err))
	}
	if _, calendar := m.formRows(); calendar {
		rows = append(rows, "", m.picker.View(m.now()), "",
			MutedStyle.Render("hjkl day/week · t today · or type the date"))
	}
	rows = append(rows, MutedStyle.Render("tab field · ctrl+j new line · enter save · esc cancel"))
	return ColumnStyle.Copy().Width(m.popupWidth()).Render(strings.Join(rows, "\n"))
}

func (m BoardModel) renderFooter() string {
	switch m.mode {
	case modeMove:
		return HelpStyle.Render("move: h/l reposition · enter drop · esc cancel")
	case modeConfirm:
		return HelpStyle.Render("delete this task? y / n")
	}
	if m.err != "" {
		return HelpStyle.Render("! " + m.err)
	}
	focusLabel := "item"
	if m.focus == focusColumn {
		focusLabel = "column"
	}
	return HelpStyle.Render(
		"focus: " + focusLabel + " (ctrl+t) · hjkl move · enter open · a add · e edit · d delete · m grab · tab switch · ? help · q quit")
}

// truncate shortens s to fit n display columns, appending an ellipsis when
// it has to cut. It measures with lipgloss.Width, not rune count: CJK and
// emoji occupy two columns each, so a rune budget silently overflows.
// ponytail: O(n^2) shrink loop on short strings (card titles/descriptions);
// fine at this size, revisit if it ever runs on long text.
func truncate(s string, n int) string {
	if n < 1 {
		n = 1
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)+"…") > n {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// Update dispatches on the current mode: modeInput handles add/edit text
// entry, modeMove handles grab-to-move, modeConfirm handles the delete
// prompt, and everything else falls through to normal navigation.
func (m BoardModel) Update(msg tea.Msg) (BoardModel, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch m.mode {
	case modeInput:
		return m.updateInput(keyMsg)
	case modeMove:
		return m.updateMove(keyMsg)
	case modeConfirm:
		return m.updateConfirm(keyMsg)
	case modeDetail:
		return m.updateDetail(keyMsg)
	}
	return m.updateNormal(keyMsg)
}

// updateNormal handles navigation and focus in the default mode.
func (m BoardModel) updateNormal(keyMsg tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.err = ""

	if keyMsg.Type == tea.KeyCtrlT {
		if m.focus == focusItem {
			m.focus = focusColumn
		} else {
			m.focus = focusItem
		}
		return m, nil
	}

	switch keyMsg.String() {
	case "h", "left":
		m.moveColumn(-1)
	case "l", "right":
		m.moveColumn(1)
	case "j", "down":
		if m.focus == focusItem {
			m.moveItem(1)
		}
	case "k", "up":
		if m.focus == focusItem {
			m.moveItem(-1)
		}
	case "g":
		if m.focus == focusItem {
			m.sel[m.col] = 0
		}
	case "G":
		if m.focus == focusItem {
			m.sel[m.col] = len(m.board.ByStatus(m.currentStatus())) - 1
		}
	case "enter":
		if _, ok := m.selectedTask(); !ok {
			return m, nil
		}
		m.focus = focusItem
		m.mode = modeDetail
	case "a":
		m.openForm("", "", "", nil)
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline)
	case "d":
		if _, ok := m.selectedTask(); !ok {
			return m, nil
		}
		m.mode = modeConfirm
	case "m":
		t, ok := m.selectedTask()
		if !ok {
			return m, nil
		}
		m.mode = modeMove
		m.focus = focusItem
		m.grabID = t.ID
		m.grabFrom = t.Status
	}
	m.clampSelection()
	return m, nil
}

// dirtyMsg signals that the board changed and should be persisted.
type dirtyMsg struct{}

func dirty() tea.Cmd { return func() tea.Msg { return dirtyMsg{} } }

func (m BoardModel) updateInput(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.Type {
	case tea.KeyEsc:
		m.closeForm()
		return m, nil

	case tea.KeyTab:
		m.focusField(1)
		return m, nil

	case tea.KeyShiftTab:
		m.focusField(-1)
		return m, nil

	case tea.KeyEnter:
		title := strings.TrimSpace(m.title.Value())
		if title == "" {
			m.err = "title must not be blank"
			return m, nil // stay in the form
		}
		deadline, err := parseDeadline(m.deadline.Value())
		if err != nil {
			m.err = err.Error()
			return m, nil // stay in the form
		}
		desc := strings.TrimSpace(m.desc.Value())

		if m.editID != "" {
			if err := m.board.Edit(m.editID, title, desc, deadline, m.now()); err != nil {
				m.err = err.Error()
				return m, nil
			}
		} else {
			m.board.Add(title, desc, deadline, m.now())
			m.col = 0
			m.sel[0] = len(m.board.ByStatus(m.board.Statuses()[0])) - 1
		}
		m.closeForm()
		m.clampSelection()
		return m, dirty()
	}

	// On the Deadline field the calendar is always on screen, so hjkl and t
	// steer it. Those letters are never legitimate input in a DD/MM/YYYY
	// field, which is what lets the calendar and the text box share the
	// keyboard without the calendar having to swallow everything.
	if m.field == fieldDeadline {
		moved := true
		switch k.String() {
		case "h":
			m.picker = m.picker.move(-1)
		case "l":
			m.picker = m.picker.move(1)
		case "k":
			m.picker = m.picker.move(-7)
		case "j":
			m.picker = m.picker.move(7)
		case "t":
			m.picker = newDatePicker(m.now())
		default:
			moved = false
		}
		if moved {
			m.deadline.SetValue(FormatDate(m.picker.cursor))
			m.deadline.CursorEnd()
			m.err = ""
			m.sizeForm() // a month with six week rows makes the calendar taller
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.field {
	case fieldTitle:
		m.title, cmd = m.title.Update(k)
	case fieldDesc:
		m.desc, cmd = m.desc.Update(k)
	case fieldDeadline:
		m.deadline, cmd = m.deadline.Update(k)
		// Typing wins over the cursor: once the text parses, the calendar
		// jumps to it. A half-typed date leaves the cursor where it was.
		m.seedPicker()
		m.sizeForm()
	}
	return m, cmd
}

// updateDetail handles the expanded-task popup: read-only, with e/d as the
// same shortcuts the board uses so the popup is a place to act from too.
func (m BoardModel) updateDetail(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.String() {
	case "esc", "enter", "q":
		m.mode = modeNormal
	case "e":
		t, ok := m.selectedTask()
		if !ok {
			m.mode = modeNormal
			return m, nil
		}
		m.openForm(t.ID, t.Title, t.Description, t.Deadline)
	case "d":
		if _, ok := m.selectedTask(); !ok {
			m.mode = modeNormal
			return m, nil
		}
		m.mode = modeConfirm
	}
	return m, nil
}

// seedPicker points the calendar at whatever the Deadline field currently
// says. A complete date wins; an empty field falls back to today. A partly
// typed date is the one case that leaves the cursor alone — snapping back to
// today on every keystroke would make the calendar jump around while you
// type. Note parseDeadline reports an empty field as (nil, nil), a valid "no
// deadline", so emptiness has to be checked separately from a parse failure.
func (m *BoardModel) seedPicker() {
	text := strings.TrimSpace(m.deadline.Value())
	d, err := parseDeadline(text)
	switch {
	case err == nil && d != nil:
		m.picker = newDatePicker(*d)
	case text == "" || !m.picker.open:
		m.picker = newDatePicker(m.now())
	}
}

func (m BoardModel) updateMove(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	switch k.String() {
	case "h", "left":
		if m.shiftGrabbed(-1) {
			return m, dirty()
		}
		return m, nil
	case "l", "right":
		if m.shiftGrabbed(1) {
			return m, dirty()
		}
		return m, nil
	case "enter":
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	case "esc":
		if err := m.board.Move(m.grabID, m.grabFrom, m.now()); err != nil {
			m.err = err.Error()
		}
		m.col = m.board.ColumnIndex(m.grabFrom)
		m.mode = modeNormal
		m.grabID = ""
		m.clampSelection()
		return m, dirty()
	}
	return m, nil
}

// shiftGrabbed moves the grabbed task one column over and follows it. It
// reports whether anything actually moved, so a no-op press (at either end
// of the board, or on a Move error) doesn't trigger a spurious dirty save.
func (m *BoardModel) shiftGrabbed(delta int) bool {
	next := m.col + delta
	if next < 0 || next >= len(m.board.Statuses()) {
		return false
	}
	if err := m.board.Move(m.grabID, m.board.Statuses()[next], m.now()); err != nil {
		m.err = err.Error()
		return false
	}
	m.col = next
	items := m.board.ByStatus(m.board.Statuses()[next])
	for i, t := range items {
		if t.ID == m.grabID {
			m.sel[next] = i
			break
		}
	}
	m.clampSelection()
	return true
}

func (m BoardModel) updateConfirm(k tea.KeyMsg) (BoardModel, tea.Cmd) {
	m.mode = modeNormal
	if k.String() != "y" {
		return m, nil
	}
	t, ok := m.selectedTask()
	if !ok {
		return m, nil
	}
	if err := m.board.Delete(t.ID); err != nil {
		m.err = err.Error()
		return m, nil
	}
	m.clampSelection()
	return m, dirty()
}

// moveColumn shifts the focused column, clamped at both edges.
func (m *BoardModel) moveColumn(delta int) {
	next := m.col + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.board.Statuses()) {
		next = len(m.board.Statuses()) - 1
	}
	m.col = next
}

// moveItem shifts the cursor within the focused column, clamped at both ends.
func (m *BoardModel) moveItem(delta int) {
	n := len(m.board.ByStatus(m.currentStatus()))
	if n == 0 {
		m.sel[m.col] = 0
		return
	}
	next := m.sel[m.col] + delta
	if next < 0 {
		next = 0
	}
	if next >= n {
		next = n - 1
	}
	m.sel[m.col] = next
}

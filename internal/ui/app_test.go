package ui

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

func app(t *testing.T) AppModel {
	t.Helper()
	a := NewApp(seeded(t))
	a.board.now = fixedClock(a.board).now
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m.(AppModel)
}

func TestTabSwitchesPages(t *testing.T) {
	a := app(t)
	if a.page != pageBoard {
		t.Fatalf("page = %v, want pageBoard", a.page)
	}
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageAnalytics {
		t.Fatalf("page = %v, want pageAnalytics", a.page)
	}
	if !strings.Contains(a.View(), "THROUGHPUT") {
		t.Errorf("analytics page view missing THROUGHPUT:\n%s", a.View())
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.page != pageArchive {
		t.Errorf("page = %v, want pageArchive after a second tab", a.page)
	}
}

func TestQuestionMarkTogglesHelp(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	if !a.showHelp {
		t.Fatal("showHelp = false, want true")
	}
	if !strings.Contains(a.View(), "ctrl+t") {
		t.Errorf("help overlay missing the ctrl+t binding:\n%s", a.View())
	}
	if !strings.Contains(a.View(), "calendar") {
		t.Errorf("help overlay does not mention the deadline calendar:\n%s", a.View())
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	if m.(AppModel).showHelp {
		t.Error("showHelp = true, want false after a second ?")
	}
}

func TestQQuits(t *testing.T) {
	a := app(t)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("q produced %T, want tea.QuitMsg", cmd())
	}
}

func TestQIsTypableWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	a = m.(AppModel)
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q quit the app while the input was open")
		}
	}
	if a.board.title.Value() != "q" {
		t.Errorf("input value = %q, want %q", a.board.title.Value(), "q")
	}
}

func TestTabIgnoredWhileAddingATask(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	a = m.(AppModel)
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.(AppModel).page != pageBoard {
		t.Error("tab switched pages while the input was open, want it ignored")
	}
}

func TestHelpOpenTabDismissesOnly(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	a = m.(AppModel)
	if a.showHelp {
		t.Error("showHelp = true, want false after tab while help open")
	}
	if a.page != pageBoard {
		t.Errorf("page = %v, want pageBoard unchanged", a.page)
	}
}

func TestHelpOpenQDismissesOnly(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	a = m.(AppModel)
	if a.showHelp {
		t.Error("showHelp = true, want false after q while help open")
	}
	if cmd != nil {
		t.Error("q while help open returned a non-nil cmd, want nil (must not quit)")
	}
}

func TestHelpOpenCtrlCStillQuits(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c while help open returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c while help open produced %T, want tea.QuitMsg", cmd())
	}
}

func TestCoalescedMultiRuneMovesGrabbedTaskTwoColumns(t *testing.T) {
	b := &task.Board{}
	b.Add("[Task title]", "", nil, ref)
	a := NewApp(b)
	a.board = fixedClock(a.board)

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	a = m.(AppModel)
	if a.board.mode != modeMove {
		t.Fatalf("mode = %v, want modeMove", a.board.mode)
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ll")})
	a = m.(AppModel)
	if got := a.board.board.Tasks[0].Status; got != task.StatusBlocked {
		t.Errorf("status after coalesced \"ll\" = %q, want blocked (todo -> doing -> blocked)", got)
	}
}

func TestCoalescedMultiRuneMovesSelectionDownTwo(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", "", nil, ref)
	b.Add("[Two]", "", nil, ref)
	b.Add("[Three]", "", nil, ref)
	a := NewApp(b)
	a.board = fixedClock(a.board)

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	a = m.(AppModel)
	if a.board.sel[0] != 2 {
		t.Errorf("sel[0] = %d, want 2 after coalesced \"jj\"", a.board.sel[0])
	}
}

func TestHelpOpenSwallowsWholeCoalescedBatch(t *testing.T) {
	b := &task.Board{}
	b.Add("[One]", "", nil, ref)
	b.Add("[Two]", "", nil, ref)
	b.Add("[Three]", "", nil, ref)
	a := NewApp(b)
	a.board = fixedClock(a.board)

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	a = m.(AppModel)
	if !a.showHelp {
		t.Fatal("showHelp = false, want true")
	}

	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jj")})
	a = m.(AppModel)
	if a.showHelp {
		t.Error("showHelp = true, want false after coalesced \"jj\" closed the overlay")
	}
	if a.board.sel[0] != 0 {
		t.Errorf("sel[0] = %d, want 0 (second rune must not reach the board)", a.board.sel[0])
	}
}

func TestQDoesNotQuitDuringDeleteConfirm(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a = m.(AppModel)
	if a.board.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm", a.board.mode)
	}
	m, cmd := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("q quit the app during delete-confirm, want it consumed by the prompt")
		}
	}
	a = m.(AppModel)
	if a.page != pageBoard {
		t.Errorf("page = %v, want pageBoard (q must not leave the board)", a.page)
	}
}

func TestTabDoesNotSwitchPagesDuringMove(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	a = m.(AppModel)
	if a.board.mode != modeMove {
		t.Fatalf("mode = %v, want modeMove", a.board.mode)
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.(AppModel).page != pageBoard {
		t.Error("tab switched pages during modeMove, want it ignored")
	}
}

func TestCtrlCStillQuitsDuringDeleteConfirm(t *testing.T) {
	a := app(t)
	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	a = m.(AppModel)
	if a.board.mode != modeConfirm {
		t.Fatalf("mode = %v, want modeConfirm", a.board.mode)
	}
	_, cmd := a.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c during delete-confirm returned a nil cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c during delete-confirm produced %T, want tea.QuitMsg", cmd())
	}
}

func TestDirtyMsgTriggersSave(t *testing.T) {
	b := &task.Board{}
	dir := t.TempDir()
	b.SetPath(dir + "/tasks.json")
	b.Add("[Task title]", "", nil, ref)

	a := NewApp(b)
	if _, cmd := a.Update(dirtyMsg{}); cmd != nil {
		cmd() // drain any follow-up
	}
	if _, err := task.Load(dir + "/tasks.json"); err != nil {
		t.Fatalf("Load after dirtyMsg returned %v", err)
	}
	reloaded, _ := task.Load(dir + "/tasks.json")
	if len(reloaded.Tasks) != 1 {
		t.Errorf("saved tasks = %d, want 1", len(reloaded.Tasks))
	}
}

func TestTabCyclesEveryPage(t *testing.T) {
	a := app(t)
	if a.page != pageBoard {
		t.Fatalf("page = %v, want pageBoard", a.page)
	}
	for _, want := range []page{pageAnalytics, pageArchive, pageCalendar, pageBoard} {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
		if a.page != want {
			t.Fatalf("page = %v, want %v", a.page, want)
		}
	}
}

func TestArchivePageRenders(t *testing.T) {
	a := app(t)
	for i := 0; i < 2; i++ {
		m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
	}
	if !strings.Contains(stripANSI(a.View()), "ARCHIVE") {
		t.Errorf("archive page did not render:\n%s", a.View())
	}
}

func TestArchiveKeysRouteToArchivePage(t *testing.T) {
	b := &task.Board{}
	for i := 0; i < 3; i++ {
		id := b.Add("[Archived task]", "", nil, ref.Add(-40*24*time.Hour)).ID
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if n := b.SweepArchive(ref); n != 3 {
		t.Fatalf("SweepArchive = %d, want 3", n)
	}

	a := NewApp(b)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = m.(AppModel)
	for i := 0; i < 2; i++ {
		m, _ = a.Update(tea.KeyMsg{Type: tea.KeyTab})
		a = m.(AppModel)
	}
	m, _ = a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	a = m.(AppModel)
	if a.archive.sel != 1 {
		t.Errorf("archive sel = %d, want 1 — j must reach the archive page", a.archive.sel)
	}
}

func TestArchiveTickSweepsAndSaves(t *testing.T) {
	b := &task.Board{}
	b.SetPath(t.TempDir() + "/tasks.json")
	id := b.Add("[Long done]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	a := NewApp(b)
	a.now = func() time.Time { return ref }
	m, cmd := a.Update(archiveTickMsg(ref))
	a = m.(AppModel)

	if !a.store.Tasks[0].Archived {
		t.Error("the tick did not archive an eligible task")
	}
	if cmd == nil {
		t.Fatal("the tick returned a nil cmd, want at least the next tick scheduled")
	}
}

func TestArchiveTickWithNothingToDoStillReschedules(t *testing.T) {
	b := &task.Board{}
	b.SetPath(t.TempDir() + "/tasks.json")
	b.Add("[Fresh]", "", nil, ref)

	a := NewApp(b)
	a.now = func() time.Time { return ref }
	_, cmd := a.Update(archiveTickMsg(ref))
	if cmd == nil {
		t.Error("cmd is nil, want the next tick rescheduled even when nothing was archived")
	}
}

// TestArchiveTickClampsBoardSelection covers a carried review item: sweeping
// can shrink the done column out from under the board's cursor. Without a
// clamp, one frame renders with the cursor pointing past the end of the
// (now shorter) column.
func TestArchiveTickClampsBoardSelection(t *testing.T) {
	b := &task.Board{}
	doneIdx := -1
	for i, s := range task.Statuses {
		if s == task.StatusDone {
			doneIdx = i
		}
	}
	var lastID string
	for i := 0; i < 3; i++ {
		lastID = b.Add("[Old done]", "", nil, ref.Add(-40*24*time.Hour)).ID
		if err := b.Move(lastID, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	_ = lastID

	a := NewApp(b)
	a.now = func() time.Time { return ref }
	a.board.col = doneIdx
	a.board.sel[doneIdx] = 2 // cursor on the last of the three done cards

	m, _ := a.Update(archiveTickMsg(ref))
	a = m.(AppModel)

	n := len(a.board.board.ByStatus(task.StatusDone))
	if a.board.sel[doneIdx] < 0 || a.board.sel[doneIdx] >= max(n, 1) {
		t.Errorf("sel[%d] = %d out of range for %d remaining done tasks", doneIdx, a.board.sel[doneIdx], n)
	}
}

// TestViewNeverExceedsTerminalHeight is the whole-frame check the 25 tests
// added alongside the date picker were missing: every component test below
// this one checks renderForm() or picker.View() in isolation, and none of
// them notice that stacking the tab bar, the board, and a form with the
// calendar open overflows the real terminal. Bubbletea's altscreen renderer
// does not clip vertically, so an overflow here means the top of the board
// scrolls off screen while the picker is open — exactly when the user needs
// to see the board they're picking a date for.
//
// The matrix runs down to 15 rows, well below a normal terminal (a tmux
// split pane is an ordinary way to land there), because the bug scaled with
// how little room the footer left, not with any one fixed height.
//
// The form is now a centred popup rather than a footer, so it no longer
// squeezes the board at all — but it can still overflow on its own, which is
// what formRows() shrinks the description box and drops the calendar to
// prevent. Nothing here is allowed to exceed the terminal.
func TestViewNeverExceedsTerminalHeight(t *testing.T) {
	for _, h := range []int{15, 18, 20, 23, 24, 40, 50} {
		a := NewApp(seeded(t))
		a.now = func() time.Time { return ref }
		a.board.now = func() time.Time { return ref }
		m, _ := a.Update(tea.WindowSizeMsg{Width: 110, Height: h})
		a = m.(AppModel)

		check := func(state string, a AppModel) {
			t.Helper()
			got := lipgloss.Height(a.View())
			if got > h {
				t.Errorf("height=%d state=%s: View is %d rows tall, overflows a %d-row terminal by %d:\n%s",
					h, state, got, h, got-h, a.View())
			}
		}

		normal := a
		check("normal", normal)
		// Item 2 regression guard: normal mode must not lose a row of board
		// space to an over-cautious reserve. footerRows is 2 in normal mode
		// (HelpStyle has PaddingTop(1)), so the pre-fix hard-coded "-6"
		// budget rendered a frame of height-1 rows; the fixed reserve must
		// still land there, not shrink the board to claim extra headroom
		// it doesn't need.
		if got := lipgloss.Height(normal.View()); got < h-1 {
			t.Errorf("height=%d: normal mode uses only %d rows, want at least %d (regression: the board lost a row)", h, got, h-1)
		}

		m2, _ := a.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
		a2 := m2.(AppModel)
		check("form open", a2)

		// Two tabs land on the Deadline field, which shows the calendar on
		// its own — it is the tallest state the form can be in.
		m3, _ := a2.Update(tea.KeyMsg{Type: tea.KeyTab})
		m3, _ = m3.(AppModel).Update(tea.KeyMsg{Type: tea.KeyTab})
		a4 := m3.(AppModel)
		if !a4.board.picker.open {
			t.Fatalf("height=%d: the calendar is not open on the Deadline field, test setup is broken", h)
		}
		check("form + calendar open", a4)
	}
}

// The tab bar names the pages on top; the way out is a footer hint on each
// page, next to the keys that live there — never a page-specific wording.
func TestTabsNamePagesOnTopWithFooterHints(t *testing.T) {
	a := app(t)
	out := stripANSI(a.View())
	lines := strings.Split(out, "\n")
	if !strings.Contains(lines[0], "BOARD") || !strings.Contains(lines[0], "CALENDAR") {
		t.Errorf("first line is not the tab bar:\n%s", out)
	}
	if strings.Contains(out, "tab to switch") {
		t.Errorf("tab bar still carries the hint:\n%s", out)
	}
	if !strings.Contains(out, "m grab · tab switch · ? help") {
		t.Errorf("board footer is missing the tab switch hint:\n%s", out)
	}
	for _, stale := range []string{"tab analytics", "tab board", "tab back to the board"} {
		if strings.Contains(out, stale) {
			t.Errorf("board view still says %q:\n%s", stale, out)
		}
	}

	m, _ := a.Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.(AppModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	m, _ = m.(AppModel).Update(tea.KeyMsg{Type: tea.KeyTab})
	out = stripANSI(m.(AppModel).View())
	if !strings.Contains(out, "t today · tab switch") {
		t.Errorf("calendar footer is missing the tab switch hint:\n%s", out)
	}
}

// An agent writing headless while the board is open appears on the next
// refresh tick: the user watches it land with no keypress.
func TestRefreshTickPicksUpExternalWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &task.Board{}
	b.SetPath(path)
	b.Add("[Mine]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	a := NewApp(b)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = m.(AppModel)

	agent, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	agent.Add("[Agent]", "", nil, ref)
	if err := agent.Save(); err != nil {
		t.Fatal(err)
	}

	m, _ = a.Update(refreshTickMsg(time.Now()))
	out := stripANSI(m.(AppModel).View())
	if !strings.Contains(out, "[Agent]") {
		t.Errorf("open board is missing the agent's card after a tick:\n%s", out)
	}
}

// A tick never yanks unsaved keystrokes away: a dirty board skips the
// refresh and converges on a later tick once saved.
func TestRefreshTickSkipsDirtyBoard(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	b := &task.Board{}
	b.SetPath(path)
	b.Add("[Mine]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	a := NewApp(b)
	m, _ := a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = m.(AppModel)

	a.store.Add("[Unsaved]", "", nil, ref)
	agent, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	agent.Add("[Agent]", "", nil, ref)
	if err := agent.Save(); err != nil {
		t.Fatal(err)
	}

	m, _ = a.Update(refreshTickMsg(time.Now()))
	out := stripANSI(m.(AppModel).View())
	if strings.Contains(out, "[Agent]") {
		t.Errorf("dirty board picked up external writes mid-typing:\n%s", out)
	}
	if !strings.Contains(out, "[Unsaved]") {
		t.Errorf("dirty board lost its own unsaved card:\n%s", out)
	}

	m, _ = m.(AppModel).Update(dirtyMsg{})
	m, _ = m.(AppModel).Update(refreshTickMsg(time.Now()))
	out = stripANSI(m.(AppModel).View())
	if !strings.Contains(out, "[Agent]") || !strings.Contains(out, "[Unsaved]") {
		t.Errorf("saved board is missing a side after converging:\n%s", out)
	}
}

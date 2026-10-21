package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gotodo/internal/task"
)

// archivedBoard returns a board with two archived tasks and one live one.
func archivedBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	due := ref.AddDate(0, 0, -20)

	older := b.Add("[Older archived]", "[first description]", &due, ref.Add(-40*24*time.Hour)).ID
	newer := b.Add("[Newer archived]", "", nil, ref.Add(-30*24*time.Hour)).ID
	for _, id := range []string{older, newer} {
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if n := b.SweepArchive(ref); n != 2 {
		t.Fatalf("SweepArchive = %d, want 2", n)
	}
	b.Add("[Still on the board]", "", nil, ref)
	return b
}

func fixedArchive(b *task.Board) ArchiveModel {
	m := NewArchiveModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestArchiveViewListsArchivedTasksOnly(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())

	for _, want := range []string{"Older archived", "Newer archived"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Still on the board") {
		t.Errorf("View shows a live task; the archive must list archived tasks only:\n%s", out)
	}
}

func TestArchiveViewShowsDescriptionAndDeadline(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "first description") {
		t.Errorf("View missing the description:\n%s", out)
	}
	if !strings.Contains(out, "10/07/2026") {
		t.Errorf("View missing the deadline date:\n%s", out)
	}
}

func TestArchiveViewShowsArchivedDate(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, FormatDate(ref)) {
		t.Errorf("View missing the archived-on date %s:\n%s", FormatDate(ref), out)
	}
}

func TestArchiveViewCountsEntries(t *testing.T) {
	out := stripANSI(fixedArchive(archivedBoard(t)).View())
	if !strings.Contains(out, "Archive (2)") {
		t.Errorf("View missing the ARCHIVE heading with its count:\n%s", out)
	}
}

func TestArchiveViewEmptyState(t *testing.T) {
	out := stripANSI(fixedArchive(&task.Board{}).View())
	if !strings.Contains(out, "Archive (0)") {
		t.Errorf("empty archive lost its heading:\n%s", out)
	}
	if !strings.Contains(out, "nothing archived yet") {
		t.Errorf("empty archive missing its placeholder:\n%s", out)
	}
}

func TestArchiveJKMovesSelection(t *testing.T) {
	m := fixedArchive(archivedBoard(t))
	if m.sel != 0 {
		t.Fatalf("sel = %d, want 0", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want 1 after j", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 1 {
		t.Errorf("sel = %d, want it clamped at the last entry", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 after k", m.sel)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want it clamped at the first entry", m.sel)
	}
}

func TestArchiveSelectionSurvivesEmptyBoard(t *testing.T) {
	m := fixedArchive(&task.Board{})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if m.sel != 0 {
		t.Errorf("sel = %d, want 0 on an empty archive", m.sel)
	}
}

// manyArchivedBoard returns n archived tasks, each rendering two lines tall.
func manyArchivedBoard(t *testing.T, n int) *task.Board {
	t.Helper()
	b := &task.Board{}
	created := ref.Add(-40 * 24 * time.Hour)
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		ids[i] = b.Add(fmt.Sprintf("[Archived task %d]", i), "", nil, created).ID
	}
	for _, id := range ids {
		if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	if got := b.SweepArchive(ref); got != n {
		t.Fatalf("SweepArchive = %d, want %d", got, n)
	}
	return b
}

// A long archive never renders taller than the terminal.
func TestArchiveViewFitsWithinHeight(t *testing.T) {
	m := NewArchiveModel(manyArchivedBoard(t, 6))
	m.now = func() time.Time { return ref }
	m.SetSize(100, 13)

	// AppModel adds the one-row tab bar above the page.
	out := stripANSI(m.View())
	if got := len(strings.Split(out, "\n")); got > 12 {
		t.Errorf("archive view is %d lines tall, want at most 12 under the tab bar:\n%s", got, out)
	}
}

// Entries that don't fit are counted in a "+N more" line.
func TestArchiveViewShowsMoreCount(t *testing.T) {
	m := NewArchiveModel(manyArchivedBoard(t, 6))
	m.now = func() time.Time { return ref }
	m.SetSize(100, 13)

	out := stripANSI(m.View())
	if !strings.Contains(out, "+4 more") {
		t.Errorf("View missing '+4 more':\n%s", out)
	}
}

// No "+N more" line when every entry fits.
func TestArchiveViewNoMoreCountWhenEverythingFits(t *testing.T) {
	m := NewArchiveModel(manyArchivedBoard(t, 6))
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)

	out := stripANSI(m.View())
	if strings.Contains(out, "more") {
		t.Errorf("View shows a '+N more' line when everything fits:\n%s", out)
	}
}

// Selecting the last entry of a long archive keeps it rendered.
func TestArchiveViewKeepsSelectedEntryVisibleBeyondFold(t *testing.T) {
	b := manyArchivedBoard(t, 6)
	m := NewArchiveModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 13)
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})

	items := b.ArchivedTasks()
	last := items[len(items)-1].Title
	out := stripANSI(m.View())
	if !strings.Contains(out, last) {
		t.Errorf("selected last entry %q should still be rendered:\n%s", last, out)
	}
}

// The deadline/archived-on meta line truncates instead of overflowing a
// narrow terminal.
func TestArchiveViewMetaLineFitsNarrowWidth(t *testing.T) {
	b := &task.Board{}
	due := ref.AddDate(0, 0, -5)
	id := b.Add(
		"[A very long archived task title that would overflow a narrow terminal]",
		"", &due, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}

	m := NewArchiveModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(30, 40)

	for _, line := range strings.Split(m.View(), "\n") {
		if got := lipgloss.Width(line); got > 30 {
			t.Errorf("line is %d columns wide, wider than the 30-wide terminal: %q", got, line)
		}
	}
}

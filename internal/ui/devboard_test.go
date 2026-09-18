package ui

import (
	"strings"
	"testing"

	"gotodo/internal/task"
)

// A dev board sizes its per-column cursor to five columns, needs about 100
// columns of terminal, and renders the pipeline headings.
func TestDevBoardRendersFiveColumns(t *testing.T) {
	b := &task.Board{Columns: task.DevColumns}
	m := NewBoardModel(b)
	if len(m.sel) != len(task.DevColumns) {
		t.Fatalf("len(sel) = %d, want %d (one cursor per column)", len(m.sel), len(task.DevColumns))
	}
	m.SetSize(200, 40)
	if got := m.minBoardWidth(); got != 100 {
		t.Errorf("minBoardWidth = %d, want 100 for five columns", got)
	}
	out := stripANSI(m.View())
	for _, want := range []string{"TODO", "IN DEV", "TESTING/REVIEW", "BLOCKED", "SHIPPED"} {
		if !strings.Contains(out, want) {
			t.Errorf("board view is missing %q:\n%s", want, out)
		}
	}
}

// An 80-column terminal fits the personal board but not the dev one.
func TestDevBoardWarnsWhenTooNarrow(t *testing.T) {
	m := NewBoardModel(&task.Board{Columns: task.DevColumns})
	m.SetSize(80, 40)
	out := stripANSI(m.View())
	if !strings.Contains(out, "terminal too narrow") {
		t.Errorf("80-column dev view should warn, got:\n%s", out)
	}
}

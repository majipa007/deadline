package ui

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"gotodo/internal/task"
)

// numericFields extracts just the numeric tokens from a rendered line, in
// order. StatTileStyle draws bordered, centered tiles, so a raw
// strings.Fields split on the tile-count row also picks up border glyphs
// (│) as their own tokens; filtering to digit-only tokens gets at the
// actual counts without weakening what's being checked — order and value
// of all four counts are still pinned exactly.
func numericFields(line string) []string {
	var out []string
	for _, f := range strings.Fields(line) {
		if _, err := strconv.Atoi(f); err == nil {
			out = append(out, f)
		}
	}
	return out
}

func analyticsBoard(t *testing.T) *task.Board {
	t.Helper()
	b := &task.Board{}
	// one done today, one done three days ago, one blocked for 9 hours
	a := b.Add("[Done today]", "", nil, ref.Add(-30*time.Hour))
	if err := b.Move(a.ID, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	c := b.Add("[Done earlier]", "", nil, ref.Add(-96*time.Hour))
	if err := b.Move(c.ID, task.StatusDone, ref.Add(-72*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	d := b.Add("[Stuck]", "", nil, ref.Add(-20*time.Hour))
	if err := b.Move(d.ID, task.StatusBlocked, ref.Add(-9*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	b.Add("[Fresh]", "", nil, ref.Add(-time.Hour))
	return b
}

func fixedAnalytics(b *task.Board) AnalyticsModel {
	m := NewAnalyticsModel(b)
	m.now = func() time.Time { return ref }
	m.SetSize(100, 40)
	return m
}

func TestAnalyticsViewHasAllSections(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, want := range []string{"THROUGHPUT", "CYCLE TIME", "BLOCKED", "STREAK"} {
		if !strings.Contains(out, want) {
			t.Errorf("View missing section %q:\n%s", want, out)
		}
	}
}

func TestAnalyticsViewShowsColumnCounts(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	for _, s := range task.Statuses {
		if !strings.Contains(out, s.Label()) {
			t.Errorf("View missing the %q tile:\n%s", s.Label(), out)
		}
	}
}

func TestAnalyticsViewUsesSingaporeDates(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "30/07/2026") {
		t.Errorf("View missing the DD/MM/YYYY end date:\n%s", out)
	}
}

func TestAnalyticsViewListsBlockedTask(t *testing.T) {
	out := fixedAnalytics(analyticsBoard(t)).View()
	if !strings.Contains(out, "Stuck") {
		t.Errorf("View missing the blocked task title:\n%s", out)
	}
	if !strings.Contains(out, "9h") {
		t.Errorf("View missing the blocked duration '9h':\n%s", out)
	}
}

func TestAnalyticsTilesExcludeArchivedButHistoryDoesNot(t *testing.T) {
	b := &task.Board{}
	id := b.Add("[Long done]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(id, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}

	out := stripANSI(fixedAnalytics(b).View())

	// The DONE tile must read 0 — the task left the board.
	lines := strings.Split(out, "\n")
	var labelIdx = -1
	for i, l := range lines {
		if strings.Contains(l, "TODO") && strings.Contains(l, "DONE") {
			labelIdx = i
			break
		}
	}
	if labelIdx < 1 {
		t.Fatalf("could not find the tile label row:\n%s", out)
	}
	if got := numericFields(lines[labelIdx-1]); len(got) != 4 || got[3] != "0" {
		t.Errorf("tile counts = %v, want the DONE tile to read 0", got)
	}

	// But history-based sections must still count the completion. Note:
	// this can't be checked via the THROUGHPUT line's 14-day window —
	// SweepArchive only fires at >=14 days since completion, and the
	// throughput window only covers the 14 most recent days, so an
	// archived task's completion date can never fall inside it (the two
	// 14-day constants are mutually exclusive by construction). Cycle
	// time has no such window, so it's the meaningful check here.
	if !strings.Contains(out, "over 1 completed") {
		t.Errorf("archiving erased the completion from cycle time:\n%s", out)
	}
}

// TestAnalyticsViewRendersTruthfulNumbers builds a fixed board and checks the
// rendered figures against hand-computed expectations, rather than the
// section-heading Contains checks above (which a hardcoded-string stub of
// View would also pass). The expected values below are computed by hand from
// the fixture, never by calling into internal/stats — otherwise this would
// just assert the renderer against itself.
func TestAnalyticsViewRendersTruthfulNumbers(t *testing.T) {
	b := &task.Board{}

	// 2 todo, 1 doing, 1 blocked, 3 done (all active — nothing archived).
	b.Add("[Todo one]", "", nil, ref)
	b.Add("[Todo two]", "", nil, ref)
	doing := b.Add("[Doing]", "", nil, ref).ID
	if err := b.Move(doing, task.StatusDoing, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	blocked := b.Add("[Stuck]", "", nil, ref.Add(-10*time.Hour)).ID
	if err := b.Move(blocked, task.StatusBlocked, ref.Add(-9*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	// Cycle times 1h, 1h, 25h -> mean 9h, median 1h (hand-computed:
	// sum=27h/3=9h; sorted [1h,1h,25h], odd count -> middle element 1h).
	// Completed on three consecutive days so streak current=3, longest=3.
	d1 := b.Add("[Done today]", "", nil, ref.Add(-1*time.Hour)).ID
	if err := b.Move(d1, task.StatusDone, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	d2 := b.Add("[Done yesterday]", "", nil, ref.Add(-25*time.Hour)).ID
	if err := b.Move(d2, task.StatusDone, ref.Add(-24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	d3 := b.Add("[Done two days ago]", "", nil, ref.Add(-73*time.Hour)).ID
	if err := b.Move(d3, task.StatusDone, ref.Add(-48*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	out := stripANSI(fixedAnalytics(b).View())
	lines := strings.Split(out, "\n")

	// Tiles: todo=2, doing=1, blocked=1, done=3, in task.Statuses order.
	labelIdx := -1
	for i, l := range lines {
		if strings.Contains(l, "TODO") && strings.Contains(l, "DONE") {
			labelIdx = i
			break
		}
	}
	if labelIdx < 1 {
		t.Fatalf("could not find the tile label row:\n%s", out)
	}
	if got := numericFields(lines[labelIdx-1]); len(got) != 4 ||
		got[0] != "2" || got[1] != "1" || got[2] != "1" || got[3] != "3" {
		t.Errorf("tile counts = %v, want [2 1 1 3]", got)
	}

	// Throughput: 3 completions, all within the 14-day window, spanning
	// 17/07/2026 (14 days before 30/07/2026) through 30/07/2026.
	if !strings.Contains(out, "3 completed over 14 days · 17/07/2026 → 30/07/2026") {
		t.Errorf("throughput line wrong, want '3 completed over 14 days · 17/07/2026 → 30/07/2026':\n%s", out)
	}

	// Cycle time: mean 9h, median 1h over 3 completed tasks. Pinning both
	// separately catches a Mean/Median swap in renderCycle.
	if !strings.Contains(out, "mean 9h · median 1h · over 3 completed") {
		t.Errorf("cycle time line wrong, want 'mean 9h · median 1h · over 3 completed':\n%s", out)
	}

	// Blocked: 1 task, blocked for 9h.
	if !strings.Contains(out, "1 currently blocked") {
		t.Errorf("blocked count line wrong:\n%s", out)
	}
	if !strings.Contains(out, "9h") {
		t.Errorf("blocked duration '9h' missing:\n%s", out)
	}

	// Streak: completions on three consecutive days ending today.
	if !strings.Contains(out, "current 3 days · longest 3 days") {
		t.Errorf("streak line wrong, want 'current 3 days · longest 3 days':\n%s", out)
	}
}

// TestAnalyticsArchivedBlockedTaskExcludedButCycleTimeStillCounts pins the
// fix for an archived-but-hand-edited task: an archived blocked task must
// not show up in the BLOCKED section (that's a present-tense claim about
// the live board), while an archived done task must still count toward
// CYCLE TIME (that's a history claim, and archiving must never erase
// history). Asserting both together stops a "fix" that filters every
// section down to Active(), which would silently break the cycle-time
// invariant covered by TestAnalyticsTilesExcludeArchivedButHistoryDoesNot.
func TestAnalyticsArchivedBlockedTaskExcludedButCycleTimeStillCounts(t *testing.T) {
	b := &task.Board{}

	blocked := b.Add("[Hand-edited stuck task]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(blocked, task.StatusBlocked, ref.Add(-30*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	done := b.Add("[Long done]", "", nil, ref.Add(-40*24*time.Hour)).ID
	if err := b.Move(done, task.StatusDone, ref.Add(-20*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	// Simulate a hand-edited tasks.json with archived:true set on a
	// blocked task — not reachable through the UI, since SweepArchive
	// only archives done tasks, but the codebase already defends against
	// hand-edited files elsewhere (Load's status coercion, archivedStamp's
	// fallback).
	for i := range b.Tasks {
		if b.Tasks[i].ID == blocked || b.Tasks[i].ID == done {
			b.Tasks[i].Archived = true
		}
	}

	out := stripANSI(fixedAnalytics(b).View())

	if strings.Contains(out, "Hand-edited stuck task") {
		t.Errorf("BLOCKED section shows an archived task:\n%s", out)
	}
	if !strings.Contains(out, "nothing is blocked") {
		t.Errorf("BLOCKED section should read empty once its only entry is archived:\n%s", out)
	}
	if !strings.Contains(out, "over 1 completed") {
		t.Errorf("archiving a done task erased it from cycle time:\n%s", out)
	}
}

func TestAnalyticsViewEmptyBoardDoesNotPanic(t *testing.T) {
	out := fixedAnalytics(&task.Board{}).View()
	if out == "" {
		t.Error("View of an empty board returned an empty string")
	}
	if !strings.Contains(out, "THROUGHPUT") {
		t.Errorf("View of an empty board is missing its sections:\n%s", out)
	}
}

// Cycle bars stretch with the terminal instead of a fixed 24 cells, so the
// section holds its own next to a full-width board.
func TestCycleBarsScaleWithTerminalWidth(t *testing.T) {
	m := NewAnalyticsModel(&task.Board{})
	if got := m.barWidth(); got != 24 {
		t.Errorf("barWidth() with unknown width = %d, want 24", got)
	}
	m.SetSize(40, 40)
	if got := m.barWidth(); got != 24 {
		t.Errorf("barWidth() at 40 columns = %d, want the 24-cell floor", got)
	}
	m.SetSize(200, 40)
	if got := m.barWidth(); got != 168 {
		t.Errorf("barWidth() at 200 columns = %d, want 168", got)
	}
}

package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDevColumns(t *testing.T) {
	want := []Status{StatusTodo, StatusInDev, StatusTestingReview, StatusBlocked, StatusShipped}
	if !equalStatuses(DevColumns, want) {
		t.Errorf("DevColumns = %v, want %v", DevColumns, want)
	}
	if got := (&Board{Columns: DevColumns}).DoneStatus(); got != StatusShipped {
		t.Errorf("dev DoneStatus() = %q, want shipped", got)
	}
	if got := StatusTestingReview.Label(); got != "TESTING/REVIEW" {
		t.Errorf("testing-review label = %q, want TESTING/REVIEW", got)
	}
}

func TestBoardStatusesDefaultsToPersonal(t *testing.T) {
	var b Board
	if got := b.Statuses(); !equalStatuses(got, PersonalColumns) {
		t.Errorf("empty board Statuses() = %v, want the personal preset", got)
	}
	if got := b.DoneStatus(); got != StatusDone {
		t.Errorf("empty board DoneStatus() = %q, want done", got)
	}
	if b.HasStatus(StatusShipped) {
		t.Error("personal board HasStatus(shipped) = true, want false")
	}
}

func TestLoadMigratesReviewAndTesting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	// A six-column file from before review/testing merged: both the task
	// status and its transition history must move to testing-review, with
	// IDs, timestamps and order untouched.
	data := `{"columns": ["todo", "indev", "review", "testing", "blocked", "shipped"],` +
		`"tasks": [` +
		`{"id": "a", "title": "r", "status": "review",` +
		` "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-02T00:00:00Z",` +
		` "history": [{"from": "indev", "to": "review", "at": "2026-01-02T00:00:00Z"}]},` +
		`{"id": "b", "title": "t", "status": "testing",` +
		` "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-03T00:00:00Z",` +
		` "history": [{"from": "review", "to": "testing", "at": "2026-01-03T00:00:00Z"}]}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if !equalStatuses(b.Statuses(), DevColumns) {
		t.Errorf("Statuses() = %v, want the five-column dev layout", b.Statuses())
	}
	for _, want := range []struct {
		id   string
		from Status
		to   Status
	}{
		{"a", StatusInDev, StatusTestingReview},
		{"b", StatusTestingReview, StatusTestingReview},
	} {
		var found *Task
		for i := range b.Tasks {
			if b.Tasks[i].ID == want.id {
				found = &b.Tasks[i]
			}
		}
		if found == nil {
			t.Fatalf("task %q missing after migration", want.id)
		}
		if found.Status != StatusTestingReview {
			t.Errorf("task %q status = %q, want testing-review", want.id, found.Status)
		}
		if len(found.History) != 1 || found.History[0].From != want.from || found.History[0].To != want.to {
			t.Errorf("task %q history = %+v, want [{from %q to %q}]", want.id, found.History, want.from, want.to)
		}
	}
	// Completion still resolves through the migrated history: the
	// testing-review task is not complete while shipped is the terminal.
	if _, ok := CompletedAt(b.Tasks[0], b.DoneStatus()); ok {
		t.Error("unshipped task reports a completion, want false")
	}
}

func TestLoadRejectsUnknownColumns(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	data := `{"columns": ["todo", "doing", "qa"], "tasks": []}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Error("Load(unknown columns) = nil, want a descriptive error")
	} else if !strings.Contains(err.Error(), "qa") {
		t.Errorf("Load error = %q, want it to name the offending columns", err)
	}
}

func TestLoadCoercesUnknownStatusToFirstColumn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	data := `{"tasks": [{"id": "a", "title": "t", "status": "hand-edited",` +
		` "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want todo (coerced to the first column)", b.Tasks[0].Status)
	}
}

func TestLoadLegacyFileReadsAsPersonal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.json")
	data := `{"tasks": [{"id": "a", "title": "t", "status": "doing",` +
		` "created_at": "2026-01-01T00:00:00Z", "updated_at": "2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := Load(path)
	if err != nil {
		t.Fatalf("Load = %v, want nil", err)
	}
	if got := b.DoneStatus(); got != StatusDone {
		t.Errorf("legacy DoneStatus() = %q, want done", got)
	}
	if b.Tasks[0].Status != StatusDoing {
		t.Errorf("Status = %q, want doing (legacy tasks keep their column)", b.Tasks[0].Status)
	}
}

func TestSweepArchiveCollectsShippedOnDevBoard(t *testing.T) {
	b := &Board{Columns: DevColumns}
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	id := b.Add("[Shipped long ago]", "", nil, now.Add(-30*24*time.Hour)).ID
	for _, s := range []Status{StatusInDev, StatusTestingReview, StatusShipped} {
		if err := b.Move(id, s, now.Add(-20*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if n := b.SweepArchive(now); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1 (shipped is the dev terminal column)", n)
	}
	if !b.Tasks[0].Archived {
		t.Error("task is not archived, want it swept off the dev board")
	}
}

func TestCompletedAtUsesBoardTerminalColumn(t *testing.T) {
	at := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	tk := Task{Status: StatusShipped,
		History: []Transition{{From: StatusTestingReview, To: StatusShipped, At: at}}}
	if _, ok := CompletedAt(tk, StatusDone); ok {
		t.Error("CompletedAt(shipped, done) = ok, want false (wrong terminal column)")
	}
	got, ok := CompletedAt(tk, StatusShipped)
	if !ok || !got.Equal(at) {
		t.Errorf("CompletedAt(shipped, shipped) = (%v, %v), want (%v, true)", got, ok, at)
	}
}

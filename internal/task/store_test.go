package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadMissingFileReturnsEmptyBoard(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v, want nil", err)
	}
	if len(b.Tasks) != 0 {
		t.Errorf("Tasks = %+v, want empty", b.Tasks)
	}
	if b.Dirty() {
		t.Error("Dirty() = true for a freshly loaded board, want false")
	}
}

func TestSaveClearsDirty(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	b.Add("[Task title]", "", nil, ref)
	if !b.Dirty() {
		t.Fatal("Dirty() = false after Add, want true")
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}
	if b.Dirty() {
		t.Error("Dirty() = true after a successful Save, want false")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	id := b.Add("[Task title]", "", nil, ref).ID
	if err := b.Move(id, StatusBlocked, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	got, err := Load(p)
	if err != nil {
		t.Fatalf("reload returned %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(got.Tasks))
	}
	if got.Tasks[0].Status != StatusBlocked {
		t.Errorf("Status = %q, want %q", got.Tasks[0].Status, StatusBlocked)
	}
	if len(got.Tasks[0].History) != 1 {
		t.Errorf("len(History) = %d, want 1", len(got.Tasks[0].History))
	}
}

// Every mutation must survive Save + reload, on both column layouts: what
// the board holds in memory is what the file must hold afterwards.
func TestSavePersistsNewTaskWithAllFields(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(DevColumns)
	dl := ref.Add(48 * time.Hour)
	id := b.Add("[Full card]", "line one\nline two", &dl, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(got.Tasks))
	}
	ts := got.Tasks[0]
	if ts.ID != id || ts.Title != "[Full card]" || ts.Description != "line one\nline two" {
		t.Errorf("task = %+v, want all fields back", ts)
	}
	if ts.Deadline == nil || !ts.Deadline.Equal(dl) {
		t.Errorf("Deadline = %v, want %v", ts.Deadline, dl)
	}
	if ts.Status != StatusTodo {
		t.Errorf("Status = %q, want todo (new tasks start in the first column)", ts.Status)
	}
	if !ts.CreatedAt.Equal(ref) || !ts.UpdatedAt.Equal(ref) {
		t.Errorf("timestamps = %v/%v, want %v", ts.CreatedAt, ts.UpdatedAt, ref)
	}
}

func TestSavePersistsEdits(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	dl := ref.Add(24 * time.Hour)
	id := b.Add("[Old]", "[Old desc]", &dl, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	later := ref.Add(time.Hour)
	if err := session.Edit(id, "[New]", "[New desc]", nil, later); err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	ts := got.Tasks[0]
	if ts.Title != "[New]" || ts.Description != "[New desc]" {
		t.Errorf("task = %+v, want edited title and description", ts)
	}
	if ts.Deadline != nil {
		t.Errorf("Deadline = %v, want nil (a nil edit clears it)", ts.Deadline)
	}
	if !ts.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", ts.UpdatedAt, later)
	}
}

func TestSavePersistsMovesThroughDevPipeline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(DevColumns)
	id := b.Add("[Pipeline]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	pipe := []Status{StatusInDev, StatusTestingReview, StatusShipped}
	for i, s := range pipe {
		if err := session.Move(id, s, ref.Add(time.Duration(i+1)*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	ts := got.Tasks[0]
	if ts.Status != StatusShipped {
		t.Errorf("Status = %q, want shipped", ts.Status)
	}
	if len(ts.History) != 3 {
		t.Fatalf("len(History) = %d, want 3", len(ts.History))
	}
	for i, s := range pipe {
		if ts.History[i].To != s {
			t.Errorf("History[%d].To = %q, want %q", i, ts.History[i].To, s)
		}
	}
	if len(got.ByStatus(StatusShipped)) != 1 {
		t.Error("reloaded board does not list the task under shipped")
	}
}

func TestSavePersistsColumns(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(DevColumns)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStatuses(got.Statuses(), DevColumns) {
		t.Errorf("Statuses() = %q, want the dev layout", columnNames(got.Statuses()))
	}
	if got.DoneStatus() != StatusShipped {
		t.Errorf("DoneStatus() = %q, want shipped", got.DoneStatus())
	}
}

func TestSavePersistsSweep(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(DevColumns)
	old := ref.Add(-30 * 24 * time.Hour)
	id := b.Add("[Ancient]", "", nil, old).ID
	for _, s := range []Status{StatusInDev, StatusTestingReview, StatusShipped} {
		if err := b.Move(id, s, old); err != nil {
			t.Fatal(err)
		}
	}
	fresh := b.Add("[Fresh]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := session.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Active()) != 1 || got.Active()[0].ID != fresh {
		t.Errorf("active = %+v, want only the fresh task", got.Active())
	}
	arch := got.ArchivedTasks()
	if len(arch) != 1 || arch[0].ID != id || !arch[0].Archived {
		t.Errorf("archived = %+v, want the swept task", arch)
	}
	if len(got.ByStatus(StatusShipped)) != 0 {
		t.Error("swept task still listed under shipped after reload")
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tasks.json")
	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v", err)
	}
	b.Add("[Task title]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatalf("Save returned %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "tasks.json" {
		t.Errorf("dir contents = %v, want only tasks.json", entries)
	}
}

func TestLoadCoercesUnknownStatusToTodo(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	body := `{"tasks":[{"id":"abc123","title":"[Ghost task]","status":"archived",` +
		`"created_at":"2026-07-30T12:00:00Z","updated_at":"2026-07-30T12:00:00Z"}]}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile returned %v", err)
	}

	b, err := Load(p)
	if err != nil {
		t.Fatalf("Load returned %v, want nil", err)
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want %q", b.Tasks[0].Status, StatusTodo)
	}
}

func TestLoadCorruptFileErrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned %v", err)
	}
	if _, err := Load(p); err == nil {
		t.Error("Load returned nil error for corrupt JSON, want an error")
	}
}

// Two sessions saving in turn must keep both sides' cards: the agent
// logging while the TUI is open is the reason Save merges.
func TestSaveMergesConcurrentAdds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	tui, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	tui.Add("[TUI card]", "", nil, ref)
	if err := tui.Save(); err != nil {
		t.Fatal(err)
	}
	agent.Add("[Agent card]", "", nil, ref)
	if err := agent.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (no lost update)", len(re.Tasks))
	}
}

// A delete must stay deleted: Save merges concurrent adds from disk, but a
// task removed this session is a tombstone, never merged back.
func TestSaveDoesNotResurrectDeletes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(DevColumns)
	drop := b.Add("[Drop]", "", nil, ref).ID
	if err := b.Move(drop, StatusTestingReview, ref); err != nil {
		t.Fatal(err)
	}
	keep := b.Add("[Keep]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Delete(drop); err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 1 || re.Tasks[0].ID != keep {
		t.Fatalf("tasks = %+v, want only the kept task", re.Tasks)
	}
}

// Deletes hold while concurrent adds still merge: the tombstone only skips
// the removed ID, everything else on disk is appended as before.
func TestSaveMergesAddsAroundDeletes(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	drop := b.Add("[Drop]", "", nil, ref).ID
	keep := b.Add("[Keep]", "", nil, ref).ID
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	session, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Delete(drop); err != nil {
		t.Fatal(err)
	}
	other, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	other.Add("[Late]", "", nil, ref)
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}
	if err := session.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 2 {
		t.Fatalf("tasks = %d, want 2 (kept + late)", len(re.Tasks))
	}
	for _, tc := range re.Tasks {
		if tc.ID == drop {
			t.Fatal("deleted task was merged back from disk")
		}
	}
	_ = keep
}

// On the same task the later save wins whole: merging is per task, not
// per field, so concurrent edits of one card resolve last-writer-wins.
func TestSaveSessionEditsWinOverDisk(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	first, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	id := first.Add("[Title]", "", nil, ref).ID
	if err := first.Save(); err != nil {
		t.Fatal(err)
	}
	mover, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	renamer, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := mover.Move(id, StatusDoing, ref); err != nil {
		t.Fatal(err)
	}
	if err := mover.Save(); err != nil {
		t.Fatal(err)
	}
	if err := renamer.Edit(id, "[Renamed]", "", nil, ref); err != nil {
		t.Fatal(err)
	}
	if err := renamer.Save(); err != nil {
		t.Fatal(err)
	}
	re, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if re.Tasks[0].Title != "[Renamed]" {
		t.Errorf("Title = %q, want [Renamed] (later save wins)", re.Tasks[0].Title)
	}
}

// A clean board with nothing new on disk writes nothing: read-only
// sessions stay read-only even with the merge step in Save.
func TestSaveCleanBoardWritesNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "board.json")
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	b.Add("[Title]", "", nil, ref)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := quiet.Save(); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("clean Save rewrote the file, want it untouched")
	}
}

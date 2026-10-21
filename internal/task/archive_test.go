package task

import (
	"testing"
	"time"
)

// completedAgo returns a done task that entered done the given duration before ref.
func completedAgo(b *Board, title string, ago time.Duration) string {
	id := b.Add(title, "", nil, ref.Add(-ago-time.Hour)).ID
	_ = b.Move(id, StatusDone, ref.Add(-ago))
	return id
}

func TestSweepArchiveMovesOldDoneTasks(t *testing.T) {
	var b Board
	old := completedAgo(&b, "[Long done]", 15*24*time.Hour)
	recent := completedAgo(&b, "[Just done]", 2*24*time.Hour)

	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("SweepArchive = %d, want 1", n)
	}
	for _, tk := range b.Tasks {
		switch tk.ID {
		case old:
			if !tk.Archived {
				t.Error("the 15-day-old task was not archived")
			}
			if tk.ArchivedAt == nil || !tk.ArchivedAt.Equal(ref) {
				t.Errorf("ArchivedAt = %v, want %v", tk.ArchivedAt, ref)
			}
		case recent:
			if tk.Archived {
				t.Error("the 2-day-old task was archived, want it left on the board")
			}
		}
	}
}

func TestSweepArchiveBoundaryIsInclusive(t *testing.T) {
	var b Board
	completedAgo(&b, "[Exactly fourteen days]", ArchiveAfter)
	if n := b.SweepArchive(ref); n != 1 {
		t.Errorf("SweepArchive = %d, want 1 at exactly the threshold", n)
	}
}

func TestSweepArchiveJustUnderThreshold(t *testing.T) {
	var b Board
	completedAgo(&b, "[One minute short]", ArchiveAfter-time.Minute)
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 just under the threshold", n)
	}
}

func TestSweepArchiveIgnoresUnfinishedTasks(t *testing.T) {
	var b Board
	id := b.Add("[Still going]", "", nil, ref.Add(-100*24*time.Hour)).ID
	if err := b.Move(id, StatusBlocked, ref.Add(-99*24*time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 for a blocked task", n)
	}
}

func TestSweepArchiveIgnoresReopenedTasks(t *testing.T) {
	var b Board
	id := completedAgo(&b, "[Reopened]", 30*24*time.Hour)
	if err := b.Move(id, StatusDoing, ref.Add(-time.Hour)); err != nil {
		t.Fatalf("Move returned %v", err)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("SweepArchive = %d, want 0 for a task moved back out of done", n)
	}
}

func TestSweepArchiveIsIdempotent(t *testing.T) {
	var b Board
	completedAgo(&b, "[Long done]", 20*24*time.Hour)
	if n := b.SweepArchive(ref); n != 1 {
		t.Fatalf("first sweep = %d, want 1", n)
	}
	if n := b.SweepArchive(ref); n != 0 {
		t.Errorf("second sweep = %d, want 0 — already archived", n)
	}
}

func TestSweepArchiveKeepsParentWithActiveSubtask(t *testing.T) {
	var b Board
	parent := b.Add("Parent", "", nil, ref.Add(-31*24*time.Hour)).ID
	child, err := b.AddChild(parent, "Subtask", "", nil, ref.Add(-31*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Move(child.ID, StatusDone, ref.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := b.Move(parent, StatusDone, ref.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// An active child under a completed parent (bad persisted state) keeps
	// the parent unarchived.
	b.Tasks[1].Status = StatusDoing
	if n := b.SweepArchive(ref); n != 0 {
		t.Fatalf("SweepArchive = %d, want parent retained", n)
	}
	got, _ := b.TaskByID(parent)
	if got.Archived {
		t.Fatal("parent with active subtask was archived")
	}
}

func TestSweepArchiveMarksBoardDirtyOnlyWhenItMoves(t *testing.T) {
	var b Board
	completedAgo(&b, "[Just done]", time.Hour)
	b.dirty = false
	if b.SweepArchive(ref); b.Dirty() {
		t.Error("board is dirty after a no-op sweep, want clean")
	}

	completedAgo(&b, "[Long done]", 30*24*time.Hour)
	b.dirty = false
	if b.SweepArchive(ref); !b.Dirty() {
		t.Error("board is clean after archiving a task, want dirty")
	}
}

package task

import (
	"errors"
	"testing"
	"time"
)

func TestBoardAddAppendsToTodo(t *testing.T) {
	var b Board
	got := b.Add("[Task title]", "", nil, ref)
	if got == nil {
		t.Fatal("Add returned nil")
	}
	if len(b.Tasks) != 1 {
		t.Fatalf("len(Tasks) = %d, want 1", len(b.Tasks))
	}
	if b.Tasks[0].Status != StatusTodo {
		t.Errorf("Status = %q, want %q", b.Tasks[0].Status, StatusTodo)
	}
}

func TestBoardMoveRecordsTransition(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", "", nil, ref).ID
	later := ref.Add(time.Hour)

	if err := b.Move(id, StatusDoing, later); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}

	got := b.Tasks[0]
	if got.Status != StatusDoing {
		t.Errorf("Status = %q, want %q", got.Status, StatusDoing)
	}
	if len(got.History) != 1 {
		t.Fatalf("len(History) = %d, want 1", len(got.History))
	}
	tr := got.History[0]
	if tr.From != StatusTodo || tr.To != StatusDoing || !tr.At.Equal(later) {
		t.Errorf("History[0] = %+v, want {todo doing %v}", tr, later)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestBoardMoveToSameStatusIsNoOp(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", "", nil, ref).ID
	if err := b.Move(id, StatusTodo, ref.Add(time.Hour)); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}
	if len(b.Tasks[0].History) != 0 {
		t.Errorf("History = %v, want empty for a same-column move", b.Tasks[0].History)
	}
}

func TestBoardMoveUnknownID(t *testing.T) {
	var b Board
	if err := b.Move("missing", StatusDone, ref); !errors.Is(err, ErrNotFound) {
		t.Errorf("Move error = %v, want ErrNotFound", err)
	}
}

func TestBoardEdit(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", "", nil, ref).ID
	later := ref.Add(time.Hour)
	if err := b.Edit(id, "[New title]", "", nil, later); err != nil {
		t.Fatalf("Edit returned %v, want nil", err)
	}
	if b.Tasks[0].Title != "[New title]" {
		t.Errorf("Title = %q, want %q", b.Tasks[0].Title, "[New title]")
	}
	if !b.Tasks[0].UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", b.Tasks[0].UpdatedAt, later)
	}
}

func TestBoardDelete(t *testing.T) {
	var b Board
	keep := b.Add("[Keep]", "", nil, ref).ID
	drop := b.Add("[Drop]", "", nil, ref).ID

	if err := b.Delete(drop); err != nil {
		t.Fatalf("Delete returned %v, want nil", err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].ID != keep {
		t.Fatalf("Tasks = %+v, want only the kept task", b.Tasks)
	}
	if err := b.Delete(drop); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete error = %v, want ErrNotFound", err)
	}
}

func TestBoardParentAndSubtaskLifecycle(t *testing.T) {
	var b Board
	parent := b.Add("Parent", "", nil, ref).ID
	child, err := b.AddChild(parent, "Child", "", nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != parent {
		t.Fatalf("ParentID = %q, want %q", child.ParentID, parent)
	}
	if got := b.Children(parent); len(got) != 1 || got[0].ID != child.ID {
		t.Fatalf("Children = %+v, want child", got)
	}
	if done, total := b.ChildProgress(parent); done != 0 || total != 1 {
		t.Fatalf("progress = %d/%d, want 0/1", done, total)
	}
	if err := b.Move(parent, StatusDone, ref); err == nil {
		t.Fatal("Move(parent to done) = nil with unfinished child")
	}
	if err := b.Move(child.ID, StatusDone, ref); err != nil {
		t.Fatal(err)
	}
	if err := b.Move(parent, StatusDone, ref); err != nil {
		t.Fatalf("Move(parent after child) = %v", err)
	}
	if err := b.Edit(child.ID, "Renamed child", "detail", nil, ref); err != nil {
		t.Fatalf("Edit child under completed parent = %v", err)
	}
	if err := b.Move(child.ID, StatusDoing, ref); err == nil {
		t.Fatal("reopen child under completed parent = nil, want error")
	}
	if done, total := b.ChildProgress(parent); done != 1 || total != 1 {
		t.Fatalf("progress = %d/%d, want 1/1", done, total)
	}
}

func TestBoardRejectsInvalidHierarchy(t *testing.T) {
	var b Board
	parent := b.Add("Parent", "", nil, ref).ID
	child, err := b.AddChild(parent, "Child", "", nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddChild(child.ID, "Grandchild", "", nil, ref); err == nil {
		t.Error("AddChild beneath child = nil, want error")
	}
	if err := b.EditWithParent(parent, child.ID, "Parent", "", nil, ref); err == nil {
		t.Error("reparent task with children = nil, want error")
	}
	if err := b.EditWithParent(child.ID, child.ID, "Child", "", nil, ref); err == nil {
		t.Error("self-parent edit = nil, want error")
	}
	if err := b.EditWithParent(child.ID, "", "Child", "", nil, ref); err != nil {
		t.Fatalf("detach child = %v", err)
	}
	if b.Tasks[1].ParentID != "" {
		t.Errorf("ParentID = %q after detach", b.Tasks[1].ParentID)
	}
}

func TestBoardDeleteParentDetachesChildren(t *testing.T) {
	var b Board
	parent := b.Add("Parent", "", nil, ref).ID
	child, err := b.AddChild(parent, "Child", "", nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	childID := child.ID
	if err := b.Delete(parent); err != nil {
		t.Fatal(err)
	}
	got, ok := b.TaskByID(childID)
	if !ok || got.ParentID != "" {
		t.Fatalf("child after parent delete = %+v, found=%v", got, ok)
	}
}

func TestBoardAddSetsDirty(t *testing.T) {
	var b Board
	if b.Dirty() {
		t.Fatal("Dirty() = true before any mutation, want false")
	}
	b.Add("[Task title]", "", nil, ref)
	if !b.Dirty() {
		t.Error("Dirty() = false after Add, want true")
	}
}

func TestBoardMoveToSameStatusDoesNotSetDirty(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", "", nil, ref).ID
	b.dirty = false // isolate Move's effect from Add's
	if err := b.Move(id, StatusTodo, ref.Add(time.Hour)); err != nil {
		t.Fatalf("Move returned %v, want nil", err)
	}
	if b.Dirty() {
		t.Error("Dirty() = true after a same-status Move, want false")
	}
}

func TestBoardEditBlankTitleDoesNotSetDirty(t *testing.T) {
	var b Board
	id := b.Add("[Task title]", "", nil, ref).ID
	b.dirty = false
	if err := b.Edit(id, "   ", "", nil, ref); err == nil {
		t.Fatal("Edit with a blank title returned nil error, want an error")
	}
	if b.Dirty() {
		t.Error("Dirty() = true after a rejected Edit, want false")
	}
}

func TestBoardEditUnknownIDDoesNotSetDirty(t *testing.T) {
	var b Board
	if err := b.Edit("missing", "[New title]", "", nil, ref); err == nil {
		t.Fatal("Edit of an unknown id returned nil error, want an error")
	}
	if b.Dirty() {
		t.Error("Dirty() = true after Edit of an unknown id, want false")
	}
}

func TestBoardDeleteUnknownIDDoesNotSetDirty(t *testing.T) {
	var b Board
	if err := b.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete error = %v, want ErrNotFound", err)
	}
	if b.Dirty() {
		t.Error("Dirty() = true after deleting an unknown id, want false")
	}
}

func TestBoardByStatusPreservesOrder(t *testing.T) {
	var b Board
	first := b.Add("[First]", "", nil, ref).ID
	second := b.Add("[Second]", "", nil, ref).ID
	third := b.Add("[Third]", "", nil, ref).ID
	if err := b.Move(second, StatusDoing, ref); err != nil {
		t.Fatalf("Move returned %v", err)
	}

	todo := b.ByStatus(StatusTodo)
	if len(todo) != 2 || todo[0].ID != first || todo[1].ID != third {
		t.Errorf("ByStatus(todo) = %+v, want [First Third]", todo)
	}
	if doing := b.ByStatus(StatusDoing); len(doing) != 1 || doing[0].ID != second {
		t.Errorf("ByStatus(doing) = %+v, want [Second]", doing)
	}
}

func TestBoardAddStoresDescriptionAndDeadline(t *testing.T) {
	var b Board
	dl := ref.AddDate(0, 0, 3)
	got := b.Add("[Task title]", "[a description]", &dl, ref)

	if got.Description != "[a description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[a description]")
	}
	if got.Deadline == nil || !got.Deadline.Equal(dl) {
		t.Errorf("Deadline = %v, want %v", got.Deadline, dl)
	}
}

func TestBoardEditReplacesAllThreeFields(t *testing.T) {
	var b Board
	first := ref.AddDate(0, 0, 1)
	id := b.Add("[Old title]", "[old description]", &first, ref).ID

	second := ref.AddDate(0, 0, 9)
	later := ref.Add(time.Hour)
	if err := b.Edit(id, "[New title]", "[new description]", &second, later); err != nil {
		t.Fatalf("Edit returned %v", err)
	}

	got := b.Tasks[0]
	if got.Title != "[New title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[New title]")
	}
	if got.Description != "[new description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[new description]")
	}
	if got.Deadline == nil || !got.Deadline.Equal(second) {
		t.Errorf("Deadline = %v, want %v", got.Deadline, second)
	}
	if !got.UpdatedAt.Equal(later) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, later)
	}
}

func TestBoardEditCanClearTheDeadline(t *testing.T) {
	var b Board
	dl := ref.AddDate(0, 0, 3)
	id := b.Add("[Task title]", "", &dl, ref).ID

	if err := b.Edit(id, "[Task title]", "", nil, ref); err != nil {
		t.Fatalf("Edit returned %v", err)
	}
	if b.Tasks[0].Deadline != nil {
		t.Errorf("Deadline = %v, want nil after clearing", b.Tasks[0].Deadline)
	}
}

func TestByStatusExcludesArchived(t *testing.T) {
	var b Board
	keep := b.Add("[Visible]", "", nil, ref).ID
	gone := b.Add("[Archived]", "", nil, ref).ID
	for _, id := range []string{keep, gone} {
		if err := b.Move(id, StatusDone, ref); err != nil {
			t.Fatalf("Move returned %v", err)
		}
	}
	for i := range b.Tasks {
		if b.Tasks[i].ID == gone {
			b.Tasks[i].Archived = true
		}
	}

	done := b.ByStatus(StatusDone)
	if len(done) != 1 || done[0].ID != keep {
		t.Errorf("ByStatus(done) = %+v, want only the non-archived task", done)
	}
}

func TestActiveAndArchivedTasksPartitionTheBoard(t *testing.T) {
	var b Board
	b.Add("[Visible]", "", nil, ref)
	archivedID := b.Add("[Archived]", "", nil, ref).ID
	for i := range b.Tasks {
		if b.Tasks[i].ID == archivedID {
			b.Tasks[i].Archived = true
		}
	}

	if got := b.Active(); len(got) != 1 || got[0].Title != "[Visible]" {
		t.Errorf("Active() = %+v, want only the visible task", got)
	}
	if got := b.ArchivedTasks(); len(got) != 1 || got[0].Title != "[Archived]" {
		t.Errorf("ArchivedTasks() = %+v, want only the archived task", got)
	}
}

func TestArchivedTasksNewestFirst(t *testing.T) {
	var b Board
	older := b.Add("[Older]", "", nil, ref).ID
	newer := b.Add("[Newer]", "", nil, ref).ID
	oldStamp, newStamp := ref.Add(-48*time.Hour), ref.Add(-2*time.Hour)
	for i := range b.Tasks {
		switch b.Tasks[i].ID {
		case older:
			b.Tasks[i].Archived, b.Tasks[i].ArchivedAt = true, &oldStamp
		case newer:
			b.Tasks[i].Archived, b.Tasks[i].ArchivedAt = true, &newStamp
		}
	}

	got := b.ArchivedTasks()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "[Newer]" || got[1].Title != "[Older]" {
		t.Errorf("order = [%s %s], want [[Newer] [Older]]", got[0].Title, got[1].Title)
	}
}

func TestSetWorktreeTrimsClearsAndSkipsNoop(t *testing.T) {
	b := &Board{}
	now := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	id := b.Add("Task", "", nil, now).ID
	if err := b.SetWorktree(id, "  ../wt  ", now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.TaskByID(id); got.Worktree != "../wt" || !got.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("worktree = %q updated = %v", got.Worktree, got.UpdatedAt)
	}
	b.SetWorktree(id, "../wt", now.Add(2*time.Hour))
	if got, _ := b.TaskByID(id); !got.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Error("unchanged worktree bumped UpdatedAt")
	}
	b.SetWorktree(id, "", now)
	if got, _ := b.TaskByID(id); got.Worktree != "" {
		t.Errorf("blank did not clear, got %q", got.Worktree)
	}
	if err := b.SetWorktree("missing", "x", now); err == nil {
		t.Error("unknown ID accepted")
	}
}

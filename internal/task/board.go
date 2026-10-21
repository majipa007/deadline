package task

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrNotFound is returned when no task matches the given ID.
var ErrNotFound = errors.New("task not found")

// Board is the whole set of tasks. Columns are derived from Task.Status
// rather than stored separately, so a move is a single field write.
type Board struct {
	Tasks []Task `json:"tasks"`
	// Columns is the left-to-right column order. Empty (older files) means
	// the personal preset; use Statuses rather than reading it directly.
	Columns []Status `json:"columns,omitempty"`

	path  string // where Save writes
	dirty bool   // unsaved mutations pending
	// deleted holds tombstones: IDs removed by Delete this session, so
	// mergeDisk never re-appends them from the disk copy at Save time.
	deleted map[string]struct{}
	// baseline is the IDs present at the last load/save; changed is the IDs
	// mutated this session. mergeDisk uses both to keep remote and local edits.
	baseline map[string]struct{}
	changed  map[string]struct{}
}

// Statuses returns the board's columns, defaulting to the personal preset
// for files that predate per-board columns.
func (b *Board) Statuses() []Status {
	if len(b.Columns) > 0 {
		return b.Columns
	}
	return Statuses
}

// DoneStatus is the board's terminal (last) column, e.g. done or shipped.
func (b *Board) DoneStatus() Status {
	cols := b.Statuses()
	return cols[len(cols)-1]
}

// ColumnIndex is the 0-based column position of s, or -1 if not on the board.
func (b *Board) ColumnIndex(s Status) int {
	for i, c := range b.Statuses() {
		if c == s {
			return i
		}
	}
	return -1
}

// HasStatus reports whether s is a column on this board.
func (b *Board) HasStatus(s Status) bool { return b.ColumnIndex(s) >= 0 }

// SetColumns replaces the column layout and marks the board dirty, so Save
// writes it (direct assignment would bypass dirty tracking).
func (b *Board) SetColumns(cols []Status) {
	b.Columns = append([]Status(nil), cols...)
	b.dirty = true
}

func (b *Board) markChanged(id string) {
	if b.changed == nil {
		b.changed = make(map[string]struct{})
	}
	b.changed[id] = struct{}{}
	b.dirty = true
}

func (b *Board) snapshot() {
	b.baseline = make(map[string]struct{}, len(b.Tasks))
	for _, t := range b.Tasks {
		b.baseline[t.ID] = struct{}{}
	}
	b.changed = nil
}

// Dirty reports whether the board has mutations not yet written by Save.
func (b *Board) Dirty() bool { return b.dirty }

// Path reports the file Save writes to ("" when unset).
func (b *Board) Path() string { return b.path }

// Add appends a new todo task. The returned pointer is only valid until the
// next append; keep the ID, not the pointer.
func (b *Board) Add(title, description string, deadline *time.Time, now time.Time) *Task {
	b.Tasks = append(b.Tasks, NewTask(
		strings.TrimSpace(title), strings.TrimSpace(description), deadline, now))
	b.markChanged(b.Tasks[len(b.Tasks)-1].ID)
	return &b.Tasks[len(b.Tasks)-1]
}

// AddChild appends a task under a top-level parent. Hierarchy is deliberately
// one level deep: a child cannot parent another task.
func (b *Board) AddChild(parentID, title, description string, deadline *time.Time, now time.Time) (*Task, error) {
	if parentID == "" {
		return nil, errors.New("parent task is required")
	}
	if err := b.validateParent("", parentID); err != nil {
		return nil, err
	}
	t := NewTask(strings.TrimSpace(title), strings.TrimSpace(description), deadline, now)
	t.ParentID = parentID
	b.Tasks = append(b.Tasks, t)
	b.markChanged(t.ID)
	return &b.Tasks[len(b.Tasks)-1], nil
}

func (b *Board) find(id string) (*Task, error) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i], nil
		}
	}
	return nil, ErrNotFound
}

// TaskByID returns a copy of one task, including archived tasks.
func (b *Board) TaskByID(id string) (Task, bool) {
	t, err := b.find(id)
	if err != nil {
		return Task{}, false
	}
	return *t, true
}

// Children returns a parent's direct children in board insertion order.
// Archived children remain included so historical progress stays accurate.
func (b *Board) Children(parentID string) []Task {
	var out []Task
	for _, t := range b.Tasks {
		if t.ParentID == parentID {
			out = append(out, t)
		}
	}
	return out
}

// ChildProgress reports terminal children and total children.
func (b *Board) ChildProgress(parentID string) (done, total int) {
	for _, t := range b.Tasks {
		if t.ParentID != parentID {
			continue
		}
		total++
		if t.Status == b.DoneStatus() {
			done++
		}
	}
	return done, total
}

// FamilyRootID returns the top-level task ID for a parent or child.
func (b *Board) FamilyRootID(t Task) string {
	if t.ParentID != "" {
		return t.ParentID
	}
	return t.ID
}

// validateParent checks a proposed exact parent ID without mutating the board.
func (b *Board) validateParent(taskID, parentID string) error {
	if parentID == "" {
		return nil
	}
	if taskID != "" && taskID == parentID {
		return errors.New("a task cannot be its own parent")
	}
	parent, err := b.find(parentID)
	if err != nil {
		return errors.New("parent task not found")
	}
	if parent.Archived {
		return errors.New("parent task is archived")
	}
	if parent.Status == b.DoneStatus() {
		return errors.New("reopen the parent before adding subtasks")
	}
	if parent.ParentID != "" {
		return errors.New("subtasks cannot have subtasks")
	}
	if taskID != "" && len(b.Children(taskID)) > 0 {
		return errors.New("a task with subtasks cannot become a subtask")
	}
	return nil
}

// Move changes a task's column and records the transition. Moving to the
// current column is a no-op.
func (b *Board) Move(id string, to Status, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Status == to {
		return nil
	}
	if t.ParentID != "" && t.Status == b.DoneStatus() && to != b.DoneStatus() {
		if parent, ok := b.TaskByID(t.ParentID); ok && parent.Status == b.DoneStatus() {
			return errors.New("reopen the parent before reopening a subtask")
		}
	}
	if to == b.DoneStatus() {
		done, total := b.ChildProgress(id)
		if done != total {
			return fmt.Errorf("finish all subtasks first (%d/%d complete)", done, total)
		}
	}
	t.History = append(t.History, Transition{From: t.Status, To: to, At: now})
	t.Status = to
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// Edit replaces the title, description and deadline. Blank titles are
// rejected; a nil deadline clears it.
func (b *Board) Edit(id, title, description string, deadline *time.Time, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	return b.EditWithParent(id, t.ParentID, title, description, deadline, now)
}

// EditWithParent replaces editable fields and assigns or clears the parent
// atomically. Callers pass an exact parent ID; an empty ID makes a root task.
func (b *Board) EditWithParent(id, parentID, title, description string, deadline *time.Time, now time.Time) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be blank")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if parentID != t.ParentID {
		if err := b.validateParent(id, parentID); err != nil {
			return err
		}
	}
	t.ParentID = parentID
	t.Title = title
	t.Description = strings.TrimSpace(description)
	t.Deadline = deadline
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// SetWorktree records where a task's work lives (a git worktree path or
// branch); blank clears it. Unchanged values are a no-op.
func (b *Board) SetWorktree(id, worktree string, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	worktree = strings.TrimSpace(worktree)
	if t.Worktree == worktree {
		return nil
	}
	t.Worktree = worktree
	t.UpdatedAt = now
	b.markChanged(id)
	return nil
}

// Delete removes a task and records a tombstone. A deleted parent's children
// are detached, not deleted.
func (b *Board) Delete(id string) error {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			for j := range b.Tasks {
				if b.Tasks[j].ParentID == id {
					b.Tasks[j].ParentID = ""
					b.markChanged(b.Tasks[j].ID)
				}
			}
			b.Tasks = append(b.Tasks[:i], b.Tasks[i+1:]...)
			if b.deleted == nil {
				b.deleted = make(map[string]struct{})
			}
			b.deleted[id] = struct{}{}
			b.dirty = true
			return nil
		}
	}
	return ErrNotFound
}

// validateHierarchy rejects malformed persisted relationships. Runtime
// mutation methods enforce the same one-level rules before writing.
func (b *Board) validateHierarchy() error {
	byID := make(map[string]Task, len(b.Tasks))
	for _, t := range b.Tasks {
		if _, exists := byID[t.ID]; exists {
			return fmt.Errorf("duplicate task id %q", t.ID)
		}
		byID[t.ID] = t
	}
	for _, t := range b.Tasks {
		if t.ParentID == "" {
			continue
		}
		if t.ParentID == t.ID {
			return fmt.Errorf("task %s is its own parent", t.ID)
		}
		parent, ok := byID[t.ParentID]
		if !ok {
			return fmt.Errorf("task %s has missing parent %s", t.ID, t.ParentID)
		}
		if parent.ParentID != "" {
			return fmt.Errorf("task %s creates hierarchy deeper than one level", t.ID)
		}
		if parent.Status == b.DoneStatus() && t.Status != b.DoneStatus() {
			return fmt.Errorf("completed parent %s has unfinished subtask %s", parent.ID, t.ID)
		}
	}
	return nil
}

// CarryTombstonesFrom copies prev's tombstones onto b; call it before
// replacing a board with a freshly loaded one.
func (b *Board) CarryTombstonesFrom(prev *Board) {
	if len(prev.deleted) == 0 {
		return
	}
	if b.deleted == nil {
		b.deleted = make(map[string]struct{}, len(prev.deleted))
	}
	for id := range prev.deleted {
		b.deleted[id] = struct{}{}
	}
}

// ByStatus returns the non-archived tasks in one column, in insertion order.
func (b *Board) ByStatus(s Status) []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Status == s && !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// Active is every task still on the board.
func (b *Board) Active() []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if !t.Archived {
			out = append(out, t)
		}
	}
	return out
}

// ArchivedTasks is every archived task, most recently archived first.
func (b *Board) ArchivedTasks() []Task {
	out := make([]Task, 0, len(b.Tasks))
	for _, t := range b.Tasks {
		if t.Archived {
			out = append(out, t)
		}
	}
	slices.SortStableFunc(out, func(x, y Task) int {
		return archivedStamp(y).Compare(archivedStamp(x)) // newest first
	})
	return out
}

// archivedStamp falls back to UpdatedAt for tasks saved without ArchivedAt.
func archivedStamp(t Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}

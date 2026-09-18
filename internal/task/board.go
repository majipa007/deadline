package task

import (
	"errors"
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
	// Columns is the board's left-to-right column order. Empty means a file
	// written before per-board columns existed, which reads as the personal
	// preset; Statuses resolves that so callers never branch on it.
	Columns []Status `json:"columns,omitempty"`

	path  string // where Save writes; unexported so it stays out of the JSON
	dirty bool   // true when there are unsaved mutations; unexported, not serialised
	// deleted is the set of task IDs removed by Delete this session.
	// mergeDisk consults it so a deleted task is not re-appended from the
	// disk copy at Save time (which would resurrect every delete).
	// Unexported, never serialised; IDs are never reused, so entries stay
	// valid for the session's lifetime.
	deleted map[string]struct{}
}

// Statuses returns the board's columns, defaulting to the personal preset
// for files that predate per-board columns.
func (b *Board) Statuses() []Status {
	if len(b.Columns) > 0 {
		return b.Columns
	}
	return Statuses
}

// DoneStatus is the board's terminal column: done on personal boards,
// shipped on dev boards. Archiving, analytics and urgency all key off this,
// never off a hard-coded status.
func (b *Board) DoneStatus() Status {
	cols := b.Statuses()
	return cols[len(cols)-1]
}

// ColumnIndex is the column position of s, 0-based. Unknown statuses report
// -1 so callers can tell "not on this board" apart from "first column".
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

// SetColumns replaces the board's column layout (used when initialising a
// new board file) and marks the board dirty. Direct assignment would bypass
// dirty tracking and a later Save would skip the write.
func (b *Board) SetColumns(cols []Status) {
	b.Columns = append([]Status(nil), cols...)
	b.dirty = true
}

// Dirty reports whether the board has mutations not yet written by Save.
func (b *Board) Dirty() bool { return b.dirty }

// Path reports the file Save writes to ("" when unset).
func (b *Board) Path() string { return b.path }

// Add appends a new todo task and returns a pointer into b.Tasks. The
// pointer is only valid until the next Add — take the ID off it, never
// retain it.
func (b *Board) Add(title, description string, deadline *time.Time, now time.Time) *Task {
	b.Tasks = append(b.Tasks, NewTask(
		strings.TrimSpace(title), strings.TrimSpace(description), deadline, now))
	b.dirty = true
	return &b.Tasks[len(b.Tasks)-1]
}

func (b *Board) find(id string) (*Task, error) {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
			return &b.Tasks[i], nil
		}
	}
	return nil, ErrNotFound
}

// Move changes a task's column and records the transition. Moving to the
// column it already occupies is a no-op, so the history stays meaningful.
func (b *Board) Move(id string, to Status, now time.Time) error {
	t, err := b.find(id)
	if err != nil {
		return err
	}
	if t.Status == to {
		return nil
	}
	t.History = append(t.History, Transition{From: t.Status, To: to, At: now})
	t.Status = to
	t.UpdatedAt = now
	b.dirty = true
	return nil
}

// Edit replaces the title, description and deadline. Blank titles are
// rejected so the board cannot grow unreadable empty cards; a nil deadline
// clears any existing one.
func (b *Board) Edit(id, title, description string, deadline *time.Time, now time.Time) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title must not be blank")
	}
	t, err := b.find(id)
	if err != nil {
		return err
	}
	t.Title = title
	t.Description = strings.TrimSpace(description)
	t.Deadline = deadline
	t.UpdatedAt = now
	b.dirty = true
	return nil
}

// Delete removes a task permanently. The ID is recorded as a tombstone so a
// later Save does not merge it back from the on-disk copy.
func (b *Board) Delete(id string) error {
	for i := range b.Tasks {
		if b.Tasks[i].ID == id {
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

// CarryTombstonesFrom copies the deleted-ID tombstones from prev onto b, so
// a wholesale reload (live refresh) does not lose track of this session's
// deletes. Call before replacing *b with a freshly loaded board.
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
// Archived tasks live on the Archive page and never appear on the board.
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

// archivedStamp falls back to UpdatedAt for a task archived by an older
// build that did not record ArchivedAt.
func archivedStamp(t Task) time.Time {
	if t.ArchivedAt != nil {
		return *t.ArchivedAt
	}
	return t.UpdatedAt
}

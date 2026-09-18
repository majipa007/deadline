// Package task holds the todo domain model and its JSON persistence.
package task

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// Status is the kanban column a task sits in.
type Status string

const (
	StatusTodo    Status = "todo"
	StatusDoing   Status = "doing"
	StatusBlocked Status = "blocked"
	StatusDone    Status = "done"

	// Development preset columns. Both presets share the first (todo)
	// and blocked columns; only the middle and terminal columns differ.
	StatusInDev         Status = "indev"
	StatusTestingReview Status = "testing-review"
	StatusShipped       Status = "shipped"
)

// Statuses lists the personal columns in left-to-right board order. It is
// the default for boards whose file predates per-board columns.
var Statuses = []Status{StatusTodo, StatusDoing, StatusBlocked, StatusDone}

// PersonalColumns is the default preset: a short personal board.
var PersonalColumns = Statuses

// DevColumns is the development preset: review and testing share one
// TESTING/REVIEW stage, whose terminal column is shipped instead of done.
var DevColumns = []Status{StatusTodo, StatusInDev, StatusTestingReview, StatusBlocked, StatusShipped}

// Label is the human-readable column heading.
func (s Status) Label() string {
	switch s {
	case StatusTodo:
		return "TODO"
	case StatusDoing:
		return "DOING"
	case StatusBlocked:
		return "BLOCKED"
	case StatusDone:
		return "DONE"
	case StatusInDev:
		return "IN DEV"
	case StatusTestingReview:
		return "TESTING/REVIEW"
	case StatusShipped:
		return "SHIPPED"
	}
	return string(s)
}

// Transition records one status change, so analytics can reconstruct history.
type Transition struct {
	From Status    `json:"from"`
	To   Status    `json:"to"`
	At   time.Time `json:"at"`
}

// Task is one card on the board.
type Task struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Deadline    *time.Time   `json:"deadline,omitempty"`
	Status      Status       `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Archived    bool         `json:"archived,omitempty"`
	ArchivedAt  *time.Time   `json:"archived_at,omitempty"`
	History     []Transition `json:"history,omitempty"`
}

// NewTask builds a task in the todo column with a fresh random ID. A nil
// deadline means the task has none; description may be empty.
func NewTask(title, description string, deadline *time.Time, now time.Time) Task {
	return Task{
		ID:          newID(),
		Title:       title,
		Description: description,
		Deadline:    deadline,
		Status:      StatusTodo,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read does not fail on any supported platform; a
		// timestamp-based fallback here would hex-encode ASCII digits and
		// collide for any two Adds in the same microsecond. An honest crash
		// beats silently minting colliding IDs.
		panic(err)
	}
	return hex.EncodeToString(b)
}

// CompletedAt reports when the task last entered the board's terminal
// column (done/shipped). ok is false unless the task is currently there,
// so reopened tasks are not counted as completions.
func CompletedAt(t Task, done Status) (time.Time, bool) {
	if t.Status != done {
		return time.Time{}, false
	}
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].To == done {
			return t.History[i].At, true
		}
	}
	return time.Time{}, false
}

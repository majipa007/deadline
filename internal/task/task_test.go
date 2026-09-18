package task

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

func TestNewTaskDefaults(t *testing.T) {
	got := NewTask("[Task title]", "", nil, ref)
	if got.Title != "[Task title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[Task title]")
	}
	if got.Status != StatusTodo {
		t.Errorf("Status = %q, want %q", got.Status, StatusTodo)
	}
	if !got.CreatedAt.Equal(ref) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, ref)
	}
	if got.ID == "" {
		t.Error("ID is empty")
	}
	if len(got.History) != 0 {
		t.Errorf("History = %v, want empty", got.History)
	}
}

func TestNewTaskIDsAreUnique(t *testing.T) {
	a := NewTask("[Task title]", "", nil, ref)
	b := NewTask("[Task title]", "", nil, ref)
	if a.ID == b.ID {
		t.Fatalf("IDs collided: %q", a.ID)
	}
}

func TestBoardColumnIndex(t *testing.T) {
	b := &Board{Columns: DevColumns}
	cases := map[Status]int{StatusTodo: 0, StatusInDev: 1, StatusTestingReview: 2, StatusBlocked: 3, StatusShipped: 4}
	for s, want := range cases {
		if got := b.ColumnIndex(s); got != want {
			t.Errorf("ColumnIndex(%q) = %d, want %d", s, got, want)
		}
	}
	if got := b.ColumnIndex(StatusDone); got != -1 {
		t.Errorf("ColumnIndex(done) on a dev board = %d, want -1", got)
	}
}

func TestCompletedAtReturnsLastDoneTransition(t *testing.T) {
	first := ref.Add(-48 * time.Hour)
	last := ref.Add(-2 * time.Hour)
	tk := Task{
		Status: StatusDone,
		History: []Transition{
			{From: StatusTodo, To: StatusDone, At: first},
			{From: StatusDone, To: StatusDoing, At: ref.Add(-24 * time.Hour)},
			{From: StatusDoing, To: StatusDone, At: last},
		},
	}
	got, ok := CompletedAt(tk, StatusDone)
	if !ok {
		t.Fatal("CompletedAt returned ok=false, want true")
	}
	if !got.Equal(last) {
		t.Errorf("CompletedAt = %v, want %v", got, last)
	}
}

func TestCompletedAtNotDone(t *testing.T) {
	tk := Task{Status: StatusDoing, History: []Transition{{From: StatusTodo, To: StatusDoing, At: ref}}}
	if _, ok := CompletedAt(tk, StatusDone); ok {
		t.Error("CompletedAt returned ok=true for a non-done task")
	}
}

func TestNewTaskCarriesDescriptionAndDeadline(t *testing.T) {
	due := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	got := NewTask("[Task title]", "[a short description]", &due, ref)

	if got.Description != "[a short description]" {
		t.Errorf("Description = %q, want %q", got.Description, "[a short description]")
	}
	if got.Deadline == nil {
		t.Fatal("Deadline is nil, want a value")
	}
	if !got.Deadline.Equal(due) {
		t.Errorf("Deadline = %v, want %v", *got.Deadline, due)
	}
	if got.Archived {
		t.Error("Archived = true, want false for a new task")
	}
	if got.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v, want nil", got.ArchivedAt)
	}
}

func TestNewTaskWithoutDeadline(t *testing.T) {
	got := NewTask("[Task title]", "", nil, ref)
	if got.Deadline != nil {
		t.Errorf("Deadline = %v, want nil", got.Deadline)
	}
	if got.Description != "" {
		t.Errorf("Description = %q, want empty", got.Description)
	}
}

func TestTaskJSONOmitsEmptyNewFields(t *testing.T) {
	data, err := json.Marshal(NewTask("[Task title]", "", nil, ref))
	if err != nil {
		t.Fatalf("Marshal returned %v", err)
	}
	for _, key := range []string{"description", "deadline", "archived", "archived_at"} {
		if strings.Contains(string(data), key) {
			t.Errorf("JSON contains %q for an empty field; want it omitted:\n%s", key, data)
		}
	}
}

func TestTaskJSONFromOlderVersionStillLoads(t *testing.T) {
	// A file written before deadlines existed must load with zero values.
	const old = `{"id":"abc","title":"[Task title]","status":"todo",
		"created_at":"2026-07-30T12:00:00Z","updated_at":"2026-07-30T12:00:00Z"}`
	var got Task
	if err := json.Unmarshal([]byte(old), &got); err != nil {
		t.Fatalf("Unmarshal returned %v", err)
	}
	if got.Title != "[Task title]" {
		t.Errorf("Title = %q, want %q", got.Title, "[Task title]")
	}
	if got.Deadline != nil || got.Description != "" || got.Archived {
		t.Errorf("new fields should be zero, got %+v", got)
	}
}

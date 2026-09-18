package task

import (
	"testing"
	"time"
)

// due builds a todo task whose deadline is the given day.
func due(day time.Time) Task {
	d := day
	return Task{Title: "[Task title]", Status: StatusTodo, CreatedAt: ref, Deadline: &d}
}

func TestDaysUntilDeadline(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want int
	}{
		{"later today", ref.Add(6 * time.Hour), 0},
		{"earlier today", ref.Add(-6 * time.Hour), 0},
		{"tomorrow", ref.AddDate(0, 0, 1), 1},
		{"in three days", ref.AddDate(0, 0, 3), 3},
		{"yesterday", ref.AddDate(0, 0, -1), -1},
		{"a week ago", ref.AddDate(0, 0, -7), -7},
		{"tomorrow at 09:00", time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC), 1},
		{"tomorrow at 23:00", time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC), 1},
	}
	for _, c := range cases {
		got, ok := DaysUntilDeadline(due(c.day), ref)
		if !ok {
			t.Errorf("%s: ok = false, want true", c.name)
			continue
		}
		if got != c.want {
			t.Errorf("%s: days = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestDaysUntilDeadlineNoDeadline(t *testing.T) {
	if _, ok := DaysUntilDeadline(Task{Status: StatusTodo}, ref); ok {
		t.Error("ok = true for a task with no deadline, want false")
	}
}

func TestDaysUntilDeadlineAcrossDSTTransition(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	now := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	deadline := time.Date(2026, 3, 10, 0, 0, 0, 0, loc)
	got, ok := DaysUntilDeadline(due(deadline), now)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if got != 2 {
		t.Errorf("days = %d, want 2", got)
	}
}

func TestDeadlineUrgencyBuckets(t *testing.T) {
	cases := []struct {
		name string
		day  time.Time
		want Urgency
	}{
		{"four days out is future", ref.AddDate(0, 0, 4), UrgencyFuture},
		{"three days out is soon", ref.AddDate(0, 0, 3), UrgencySoon},
		{"two days out is soon", ref.AddDate(0, 0, 2), UrgencySoon},
		{"tomorrow is urgent", ref.AddDate(0, 0, 1), UrgencyUrgent},
		{"today is urgent", ref.Add(2 * time.Hour), UrgencyUrgent},
		{"yesterday is overdue", ref.AddDate(0, 0, -1), UrgencyOverdue},
	}
	for _, c := range cases {
		if got := DeadlineUrgency(due(c.day), ref, StatusDone); got != c.want {
			t.Errorf("%s: urgency = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestDeadlineUrgencyNoDeadline(t *testing.T) {
	if got := DeadlineUrgency(Task{Status: StatusTodo}, ref, StatusDone); got != UrgencyNone {
		t.Errorf("urgency = %v, want UrgencyNone", got)
	}
}

func TestDeadlineUrgencyDoneTaskIsNeverUrgent(t *testing.T) {
	overdue := due(ref.AddDate(0, 0, -30))
	overdue.Status = StatusDone
	if got := DeadlineUrgency(overdue, ref, StatusDone); got != UrgencyDone {
		t.Errorf("urgency = %v, want UrgencyDone for a completed task", got)
	}
}

func TestDeadlineUrgencyDoneWithoutDeadlineIsNone(t *testing.T) {
	done := Task{Status: StatusDone, CreatedAt: ref}
	if got := DeadlineUrgency(done, ref, StatusDone); got != UrgencyNone {
		t.Errorf("urgency = %v, want UrgencyNone", got)
	}
}

func TestStartOfDayUsesCalendarNotElapsedTime(t *testing.T) {
	got := StartOfDay(time.Date(2026, 7, 30, 23, 59, 59, 0, time.UTC))
	want := time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("StartOfDay = %v, want %v", got, want)
	}
}

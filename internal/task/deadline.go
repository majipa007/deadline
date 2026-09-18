package task

import "time"

// Urgency is how much pressure a task's deadline is under, right now.
type Urgency int

const (
	UrgencyNone    Urgency = iota // no deadline set
	UrgencyDone                   // task is done; the deadline is history, not pressure
	UrgencyFuture                 // more than 3 days away
	UrgencySoon                   // 2 to 3 days away
	UrgencyUrgent                 // due today or tomorrow
	UrgencyOverdue                // the deadline day has passed
)

// Thresholds in whole calendar days remaining.
const (
	soonWithin   = 3 // at or under this many days: soon
	urgentWithin = 1 // at or under this many days: urgent
)

// StartOfDay truncates to midnight in t's own location. time.Truncate is
// wrong here: it operates on absolute time, not calendar days.
func StartOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// daysBetween counts whole calendar days from one midnight to another.
// Both are re-anchored to noon UTC first: UTC has no DST, and noon leaves
// twelve hours of slack, so a 23- or 25-hour calendar day cannot round the
// result off by one the way dividing an elapsed duration by 24 hours does.
func daysBetween(from, to time.Time) int {
	y1, m1, d1 := from.Date()
	y2, m2, d2 := to.Date()
	a := time.Date(y1, m1, d1, 12, 0, 0, 0, time.UTC)
	b := time.Date(y2, m2, d2, 12, 0, 0, 0, time.UTC)
	return int(b.Sub(a).Hours() / 24)
}

// DaysUntilDeadline is the number of whole calendar days from now's day to
// the deadline's day: 0 means today, 1 tomorrow, negative means past. ok is
// false when the task has no deadline.
func DaysUntilDeadline(t Task, now time.Time) (int, bool) {
	if t.Deadline == nil {
		return 0, false
	}
	from := StartOfDay(now)
	to := StartOfDay(t.Deadline.In(now.Location()))
	return daysBetween(from, to), true
}

// DeadlineUrgency buckets a task's deadline for display. A task in the
// board's terminal column always reports UrgencyDone: finishing late is not
// an ongoing emergency.
func DeadlineUrgency(t Task, now time.Time, done Status) Urgency {
	days, ok := DaysUntilDeadline(t, now)
	if !ok {
		return UrgencyNone
	}
	if t.Status == done {
		return UrgencyDone
	}
	switch {
	case days < 0:
		return UrgencyOverdue
	case days <= urgentWithin:
		return UrgencyUrgent
	case days <= soonWithin:
		return UrgencySoon
	}
	return UrgencyFuture
}

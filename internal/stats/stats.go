// Package stats computes board analytics. Functions take an explicit now so
// results are deterministic.
package stats

import (
	"slices"
	"time"

	"gotodo/internal/task"
)

// DayCount is one bucket of the throughput series.
type DayCount struct {
	Day time.Time
	N   int
}

// startOfDay truncates to midnight in t's own location (time.Truncate works
// on absolute time, not calendar days).
func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// Counts returns the number of tasks per column, zeroes included.
func Counts(tasks []task.Task, columns []task.Status) map[task.Status]int {
	out := make(map[task.Status]int, len(columns))
	for _, s := range columns {
		out[s] = 0
	}
	for _, t := range tasks {
		if _, ok := out[t.Status]; ok {
			out[t.Status]++
		}
	}
	return out
}

// CompletionsByDay buckets completed tasks by the day (midnight in loc) they
// reached the terminal column. Empty days are absent.
func CompletionsByDay(tasks []task.Task, loc *time.Location, done task.Status) map[time.Time]int {
	out := map[time.Time]int{}
	for _, t := range tasks {
		at, ok := task.CompletedAt(t, done)
		if !ok {
			continue
		}
		out[startOfDay(at.In(loc))]++
	}
	return out
}

// DeadlinesByDay groups tasks with a deadline by due day (midnight in loc).
func DeadlinesByDay(tasks []task.Task, loc *time.Location) map[time.Time][]task.Task {
	out := map[time.Time][]task.Task{}
	for _, t := range tasks {
		if t.Deadline == nil {
			continue
		}
		day := startOfDay(t.Deadline.In(loc))
		out[day] = append(out[day], t)
	}
	return out
}

// ByDeadline returns the tasks that have a deadline, soonest first; ties keep
// board order.
func ByDeadline(tasks []task.Task) []task.Task {
	var out []task.Task
	for _, t := range tasks {
		if t.Deadline != nil {
			out = append(out, t)
		}
	}
	slices.SortStableFunc(out, func(a, b task.Task) int { return a.Deadline.Compare(*b.Deadline) })
	return out
}

// Throughput returns completions per day for the last `days` days, oldest
// first, ending on now's day. Empty days are present with N == 0.
func Throughput(tasks []task.Task, days int, now time.Time, done task.Status) []DayCount {
	if days < 1 {
		return nil
	}
	byDay := CompletionsByDay(tasks, now.Location(), done)
	today := startOfDay(now)
	out := make([]DayCount, 0, days)
	for i := days - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		out = append(out, DayCount{Day: d, N: byDay[d]})
	}
	return out
}

// Streak reports the current and longest runs of consecutive days with a
// completion. The current streak may end yesterday, since today isn't over.
func Streak(tasks []task.Task, now time.Time, done task.Status) (current, longest int) {
	byDay := CompletionsByDay(tasks, now.Location(), done)
	if len(byDay) == 0 {
		return 0, 0
	}
	today := startOfDay(now)

	// Current: walk back from today, tolerating an empty today.
	start := today
	if byDay[start] == 0 {
		start = today.AddDate(0, 0, -1)
	}
	for d := start; byDay[d] > 0; d = d.AddDate(0, 0, -1) {
		current++
	}

	// Longest: walk back far enough to cover the oldest completion.
	oldest := today
	for d := range byDay {
		if d.Before(oldest) {
			oldest = d
		}
	}
	run := 0
	for d := oldest; !d.After(today); d = d.AddDate(0, 0, 1) {
		if byDay[d] > 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return current, longest
}

// TimeInStatus replays a task's history to get time spent per column. The
// terminal column does not accrue time.
func TimeInStatus(t task.Task, now time.Time, done task.Status) map[task.Status]time.Duration {
	out := map[task.Status]time.Duration{}
	var cur task.Status
	if len(t.History) > 0 {
		cur = t.History[0].From
	} else {
		cur = t.Status
	}
	start := t.CreatedAt
	for _, tr := range t.History {
		out[cur] += tr.At.Sub(start)
		cur, start = tr.To, tr.At
	}
	if cur != done {
		out[cur] += now.Sub(start)
	}
	return out
}

// CycleStats summarises how long completed tasks took end to end.
type CycleStats struct {
	Mean      time.Duration
	Median    time.Duration
	N         int
	PerColumn map[task.Status]time.Duration // mean time per column, completed tasks only
}

// CycleTimes measures created-to-terminal duration over completed tasks.
func CycleTimes(tasks []task.Task, now time.Time, done task.Status) CycleStats {
	out := CycleStats{PerColumn: map[task.Status]time.Duration{}}
	var durations []time.Duration
	totals := map[task.Status]time.Duration{}

	for _, t := range tasks {
		at, ok := task.CompletedAt(t, done)
		if !ok {
			continue
		}
		durations = append(durations, at.Sub(t.CreatedAt))
		for s, d := range TimeInStatus(t, now, done) {
			totals[s] += d
		}
	}
	out.N = len(durations)
	if out.N == 0 {
		return out
	}

	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	out.Mean = sum / time.Duration(out.N)
	out.Median = median(durations)
	for s, total := range totals {
		out.PerColumn[s] = total / time.Duration(out.N)
	}
	return out
}

func median(d []time.Duration) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	n := len(s)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// BlockedItem is one currently-blocked task and how long it has been stuck.
type BlockedItem struct {
	Title string
	For   time.Duration
}

// BlockedReport lists currently-blocked tasks, longest-blocked first.
func BlockedReport(tasks []task.Task, now time.Time) []BlockedItem {
	var out []BlockedItem
	for _, t := range tasks {
		if t.Status != task.StatusBlocked {
			continue
		}
		since := t.CreatedAt
		for i := len(t.History) - 1; i >= 0; i-- {
			if t.History[i].To == task.StatusBlocked {
				since = t.History[i].At
				break
			}
		}
		out = append(out, BlockedItem{Title: t.Title, For: now.Sub(since)})
	}
	slices.SortStableFunc(out, func(a, b BlockedItem) int {
		switch {
		case a.For > b.For:
			return -1
		case a.For < b.For:
			return 1
		}
		return 0
	})
	return out
}

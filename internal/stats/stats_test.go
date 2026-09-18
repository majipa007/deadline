package stats

import (
	"testing"
	"time"

	"gotodo/internal/task"
)

var ref = time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

// done builds a task completed at the given time.
func done(id string, created, completed time.Time) task.Task {
	return task.Task{
		ID:        id,
		Title:     "[Task title]",
		Status:    task.StatusDone,
		CreatedAt: created,
		UpdatedAt: completed,
		History:   []task.Transition{{From: task.StatusTodo, To: task.StatusDone, At: completed}},
	}
}

func TestCounts(t *testing.T) {
	tasks := []task.Task{
		{Status: task.StatusTodo}, {Status: task.StatusTodo},
		{Status: task.StatusBlocked},
	}
	got := Counts(tasks, task.Statuses)
	if got[task.StatusTodo] != 2 {
		t.Errorf("todo = %d, want 2", got[task.StatusTodo])
	}
	if got[task.StatusBlocked] != 1 {
		t.Errorf("blocked = %d, want 1", got[task.StatusBlocked])
	}
	if got[task.StatusDoing] != 0 {
		t.Errorf("doing = %d, want 0", got[task.StatusDoing])
	}
	if len(got) != len(task.Statuses) {
		t.Errorf("len(Counts) = %d, want %d (every column present)", len(got), len(task.Statuses))
	}
}

func TestThroughputZeroFillsAndOrders(t *testing.T) {
	tasks := []task.Task{
		done("a", ref.Add(-72*time.Hour), ref),                    // today
		done("b", ref.Add(-72*time.Hour), ref),                    // today
		done("c", ref.Add(-72*time.Hour), ref.Add(-48*time.Hour)), // two days ago
	}
	got := Throughput(tasks, 3, ref, task.StatusDone)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	want := []int{1, 0, 2} // oldest first: -2d, -1d, today
	for i, w := range want {
		if got[i].N != w {
			t.Errorf("day %d (%v) N = %d, want %d", i, got[i].Day, got[i].N, w)
		}
	}
	if !got[2].Day.Equal(time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last day = %v, want 30/07/2026 midnight", got[2].Day)
	}
}

func TestThroughputIgnoresUnfinished(t *testing.T) {
	tasks := []task.Task{{Status: task.StatusDoing, CreatedAt: ref, UpdatedAt: ref}}
	got := Throughput(tasks, 2, ref, task.StatusDone)
	for _, d := range got {
		if d.N != 0 {
			t.Errorf("day %v N = %d, want 0", d.Day, d.N)
		}
	}
}

func TestStreakCurrentAndLongest(t *testing.T) {
	day := func(offset int) time.Time { return ref.AddDate(0, 0, -offset) }
	tasks := []task.Task{
		done("a", day(10), day(0)), // today
		done("b", day(10), day(1)),
		done("c", day(10), day(2)),
		// gap at day 3
		done("d", day(10), day(5)),
		done("e", day(10), day(6)),
	}
	current, longest := Streak(tasks, ref, task.StatusDone)
	if current != 3 {
		t.Errorf("current = %d, want 3", current)
	}
	if longest != 3 {
		t.Errorf("longest = %d, want 3", longest)
	}
}

func TestStreakCountsYesterdayWhenTodayEmpty(t *testing.T) {
	day := func(offset int) time.Time { return ref.AddDate(0, 0, -offset) }
	tasks := []task.Task{done("a", day(5), day(1)), done("b", day(5), day(2))}
	current, _ := Streak(tasks, ref, task.StatusDone)
	if current != 2 {
		t.Errorf("current = %d, want 2 (today is not over yet)", current)
	}
}

func TestStreakEmpty(t *testing.T) {
	current, longest := Streak(nil, ref, task.StatusDone)
	if current != 0 || longest != 0 {
		t.Errorf("Streak(nil) = (%d, %d), want (0, 0)", current, longest)
	}
}

func TestTimeInStatusWalksHistory(t *testing.T) {
	created := ref.Add(-10 * time.Hour)
	tk := task.Task{
		Status:    task.StatusDoing,
		CreatedAt: created,
		History: []task.Transition{
			{From: task.StatusTodo, To: task.StatusDoing, At: created.Add(2 * time.Hour)},
			{From: task.StatusDoing, To: task.StatusBlocked, At: created.Add(3 * time.Hour)},
			{From: task.StatusBlocked, To: task.StatusDoing, At: created.Add(7 * time.Hour)},
		},
	}
	got := TimeInStatus(tk, ref, task.StatusDone)
	want := map[task.Status]time.Duration{
		task.StatusTodo:    2 * time.Hour,
		task.StatusDoing:   1*time.Hour + 3*time.Hour, // 1h before block, 3h after, still open
		task.StatusBlocked: 4 * time.Hour,
	}
	for s, w := range want {
		if got[s] != w {
			t.Errorf("TimeInStatus[%q] = %v, want %v", s, got[s], w)
		}
	}
}

func TestTimeInStatusStopsAccruingWhenDone(t *testing.T) {
	created := ref.Add(-10 * time.Hour)
	tk := task.Task{
		Status:    task.StatusDone,
		CreatedAt: created,
		History:   []task.Transition{{From: task.StatusTodo, To: task.StatusDone, At: created.Add(4 * time.Hour)}},
	}
	got := TimeInStatus(tk, ref, task.StatusDone)
	if got[task.StatusTodo] != 4*time.Hour {
		t.Errorf("todo = %v, want 4h", got[task.StatusTodo])
	}
	if got[task.StatusDone] != 0 {
		t.Errorf("done = %v, want 0 (done tasks stop accruing)", got[task.StatusDone])
	}
}

func TestCycleTimesMeanAndMedian(t *testing.T) {
	tasks := []task.Task{
		done("a", ref.Add(-2*time.Hour), ref),
		done("b", ref.Add(-4*time.Hour), ref),
		done("c", ref.Add(-9*time.Hour), ref),
		{Status: task.StatusDoing, CreatedAt: ref.Add(-100 * time.Hour)}, // ignored
	}
	got := CycleTimes(tasks, ref, task.StatusDone)
	if got.N != 3 {
		t.Fatalf("N = %d, want 3", got.N)
	}
	if got.Mean != 5*time.Hour {
		t.Errorf("Mean = %v, want 5h", got.Mean)
	}
	if got.Median != 4*time.Hour {
		t.Errorf("Median = %v, want 4h", got.Median)
	}
}

func TestCycleTimesEmpty(t *testing.T) {
	got := CycleTimes(nil, ref, task.StatusDone)
	if got.N != 0 || got.Mean != 0 || got.Median != 0 {
		t.Errorf("CycleTimes(nil) = %+v, want zeroes", got)
	}
	if got.PerColumn == nil {
		t.Error("PerColumn is nil, want an empty non-nil map")
	}
}

func TestBlockedReportSortedLongestFirst(t *testing.T) {
	mk := func(title string, blockedAt time.Time) task.Task {
		return task.Task{
			Title:     title,
			Status:    task.StatusBlocked,
			CreatedAt: blockedAt.Add(-time.Hour),
			History:   []task.Transition{{From: task.StatusDoing, To: task.StatusBlocked, At: blockedAt}},
		}
	}
	tasks := []task.Task{
		mk("[Short]", ref.Add(-1*time.Hour)),
		mk("[Long]", ref.Add(-9*time.Hour)),
		{Title: "[Not blocked]", Status: task.StatusDoing, CreatedAt: ref.Add(-99 * time.Hour)},
	}
	got := BlockedReport(tasks, ref)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Title != "[Long]" || got[0].For != 9*time.Hour {
		t.Errorf("got[0] = %+v, want {[Long] 9h}", got[0])
	}
	if got[1].Title != "[Short]" || got[1].For != 1*time.Hour {
		t.Errorf("got[1] = %+v, want {[Short] 1h}", got[1])
	}
}

// dated builds an open task due at the given time.
func dated(title string, due time.Time) task.Task {
	return task.Task{Title: title, Status: task.StatusTodo, CreatedAt: ref, Deadline: &due}
}

func TestCompletionsByDayBucketsByCalendarDay(t *testing.T) {
	day := time.Date(2026, 7, 28, 0, 0, 0, 0, time.UTC)
	tasks := []task.Task{
		done("a", ref, day.Add(9*time.Hour)),
		done("b", ref, day.Add(23*time.Hour)), // same day, much later
		done("c", ref, day.AddDate(0, 0, -1)),
		{Status: task.StatusTodo, CreatedAt: ref}, // never completed
	}
	got := CompletionsByDay(tasks, time.UTC, task.StatusDone)

	if got[day] != 2 {
		t.Errorf("28/07 = %d, want 2", got[day])
	}
	if got[day.AddDate(0, 0, -1)] != 1 {
		t.Errorf("27/07 = %d, want 1", got[day.AddDate(0, 0, -1)])
	}
	if _, ok := got[day.AddDate(0, 0, 1)]; ok {
		t.Error("29/07 is present, want empty days left out")
	}
}

func TestDeadlinesByDayGroupsAndSkipsUndated(t *testing.T) {
	day := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	tasks := []task.Task{
		dated("[One]", day.Add(6*time.Hour)),
		dated("[Two]", day.Add(20*time.Hour)),
		dated("[Three]", day.AddDate(0, 0, 3)),
		{Title: "[No deadline]", Status: task.StatusTodo},
	}
	got := DeadlinesByDay(tasks, time.UTC)

	if len(got[day]) != 2 {
		t.Errorf("12/08 has %d tasks, want 2", len(got[day]))
	}
	if len(got) != 2 {
		t.Errorf("days = %d, want 2 (the undated task must not make a bucket)", len(got))
	}
}

func TestByDeadlineSortsSoonestFirst(t *testing.T) {
	late := ref.AddDate(0, 0, 10)
	soon := ref.AddDate(0, 0, -2)
	mid := ref.AddDate(0, 0, 1)
	got := ByDeadline([]task.Task{
		dated("[Late]", late),
		{Title: "[No deadline]", Status: task.StatusTodo},
		dated("[Soon]", soon),
		dated("[Mid]", mid),
	})

	want := []string{"[Soon]", "[Mid]", "[Late]"}
	if len(got) != len(want) {
		t.Fatalf("got %d tasks, want %d (undated ones dropped)", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Title != w {
			t.Errorf("position %d = %q, want %q", i, got[i].Title, w)
		}
	}
}

package task

import "time"

// ArchiveAfter is how long a task stays in the terminal column before it is
// swept off the board into the archive.
const ArchiveAfter = 14 * 24 * time.Hour

// SweepArchive archives every task in the board's terminal column (done on
// personal boards, shipped on dev boards) that has sat there for at least
// ArchiveAfter, and reports how many it moved. It is idempotent: an already
// archived task is skipped. Tasks reopened out of the terminal column are
// never archived, because CompletedAt only reports a time for tasks
// currently in it.
func (b *Board) SweepArchive(now time.Time) int {
	moved := 0
	done := b.DoneStatus()
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if t.Archived || t.Status != done {
			continue
		}
		at, ok := CompletedAt(*t, done)
		if !ok || now.Sub(at) < ArchiveAfter {
			continue
		}
		stamp := now
		t.Archived = true
		t.ArchivedAt = &stamp
		t.UpdatedAt = now
		moved++
	}
	if moved > 0 {
		b.dirty = true
	}
	return moved
}

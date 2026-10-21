package task

import "time"

// ArchiveAfter is how long a task stays in the terminal column before it is
// swept off the board into the archive.
const ArchiveAfter = 14 * 24 * time.Hour

// SweepArchive archives tasks that have sat in the terminal column for at
// least ArchiveAfter (with all subtasks done) and reports how many it moved.
func (b *Board) SweepArchive(now time.Time) int {
	moved := 0
	done := b.DoneStatus()
	for i := range b.Tasks {
		t := &b.Tasks[i]
		if t.Archived || t.Status != done {
			continue
		}
		finished, total := b.ChildProgress(t.ID)
		if finished != total {
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
		b.markChanged(t.ID)
		moved++
	}
	return moved
}

package task

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPath is the per-user tasks file, e.g. ~/.config/gotodo/tasks.json.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate config dir: %w", err)
	}
	return filepath.Join(dir, "gotodo", "tasks.json"), nil
}

// SetPath points the board at a file for later Save calls.
func (b *Board) SetPath(p string) { b.path = p }

// Load reads the board from disk. A missing file yields an empty board;
// malformed JSON is an error so a bad file is never overwritten.
func Load(path string) (*Board, error) {
	b := &Board{path: path}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := b.normalize(); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	b.snapshot()
	return b, nil
}

// normalize migrates and validates a loaded board: legacy review/testing
// statuses become testing-review, unknown column layouts are rejected, and
// unknown task statuses fall back to the first column so they stay reachable.
func (b *Board) normalize() error {
	for i := range b.Tasks {
		if b.Tasks[i].Status == Status("review") || b.Tasks[i].Status == Status("testing") {
			b.Tasks[i].Status = StatusTestingReview
		}
		for j := range b.Tasks[i].History {
			if b.Tasks[i].History[j].From == Status("review") || b.Tasks[i].History[j].From == Status("testing") {
				b.Tasks[i].History[j].From = StatusTestingReview
			}
			if b.Tasks[i].History[j].To == Status("review") || b.Tasks[i].History[j].To == Status("testing") {
				b.Tasks[i].History[j].To = StatusTestingReview
			}
		}
	}
	// Legacy six-column dev layout upgrades to the five-column one.
	legacyDev := []Status{StatusTodo, StatusInDev, Status("review"), Status("testing"), StatusBlocked, StatusShipped}
	if equalStatuses(b.Columns, legacyDev) {
		b.Columns = append([]Status(nil), DevColumns...)
	}
	if len(b.Columns) > 0 && !equalStatuses(b.Columns, PersonalColumns) && !equalStatuses(b.Columns, DevColumns) {
		return fmt.Errorf("unknown columns %q (want personal or dev layout)", columnNames(b.Columns))
	}
	for i := range b.Tasks {
		if !b.HasStatus(b.Tasks[i].Status) {
			b.Tasks[i].Status = b.Statuses()[0]
		}
	}
	return b.validateHierarchy()
}

func equalStatuses(a, b []Status) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func columnNames(cols []Status) string {
	names := make([]string, 0, len(cols))
	for _, c := range cols {
		names = append(names, string(c))
	}
	return strings.Join(names, ", ")
}

// mergeDisk pulls in tasks written to the file since this board was loaded.
// Callers must hold the board lock. Locally changed tasks win on ID conflicts.
func (b *Board) mergeDisk() error {
	disk, err := Load(b.path)
	if err != nil {
		return err
	}
	diskByID := make(map[string]Task, len(disk.Tasks))
	for _, t := range disk.Tasks {
		diskByID[t.ID] = t
	}
	merged := make([]Task, 0, len(b.Tasks)+len(disk.Tasks))
	known := make(map[string]bool, cap(merged))
	changed := false
	for _, local := range b.Tasks {
		diskTask, onDisk := diskByID[local.ID]
		_, existed := b.baseline[local.ID]
		_, localChange := b.changed[local.ID]
		if existed && !onDisk {
			// Deleted by another writer: deletion wins over local edits.
			if b.deleted == nil {
				b.deleted = make(map[string]struct{})
			}
			b.deleted[local.ID] = struct{}{}
			changed = true
			continue
		}
		if onDisk && !localChange {
			merged = append(merged, diskTask)
		} else {
			merged = append(merged, local)
		}
		known[local.ID] = true
	}
	for _, diskTask := range disk.Tasks {
		if _, gone := b.deleted[diskTask.ID]; gone {
			continue
		}
		if !known[diskTask.ID] {
			merged = append(merged, diskTask)
			changed = true
		}
	}
	// Detach children whose parent this session deleted.
	for i := range merged {
		if _, gone := b.deleted[merged[i].ParentID]; gone {
			merged[i].ParentID = ""
			changed = true
		}
	}
	candidate := *b
	candidate.Tasks = merged
	if err := candidate.validateHierarchy(); err != nil {
		return err
	}
	b.Tasks = merged
	if changed {
		b.dirty = true
	}
	return nil
}

// Save merges in concurrent writers' changes under the board lock, then
// writes atomically via a unique temp file and rename. A clean board with
// nothing new on disk writes nothing.
func (b *Board) Save() error {
	if b.path == "" {
		return errors.New("board has no path; call SetPath or Load first")
	}
	lock, err := LockBoard(b.path)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := b.mergeDisk(); err != nil {
		return err
	}
	if !b.dirty {
		return nil
	}
	dir := filepath.Dir(b.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("encode board: %w", err)
	}
	f, err := os.CreateTemp(dir, "tasks-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := os.Rename(tmp, b.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename temp: %w", err)
	}
	b.dirty = false
	b.snapshot()
	return nil
}

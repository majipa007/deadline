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

// Load reads the board from disk. A missing file yields an empty board so
// the first run is not an error; malformed JSON is an error so a bad file is
// never silently overwritten with an empty board.
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
	return b, nil
}

// normalize migrates and validates a freshly unmarshalled board. Files
// written before review/testing merged into testing-review are upgraded
// losslessly (status and transition history); files whose columns match
// neither known schema are rejected rather than guessed at. Genuinely
// unknown task statuses still coerce to the first column so a hand-edited
// task stays reachable — the same rule the pre-column format used.
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
	// The pre-merge six-column dev layout upgrades to the five-column one.
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
	return nil
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

// mergeDisk pulls in tasks written to the file since this board was
// loaded. Callers must hold the board lock. A missing file means a fresh
// board with nothing to merge; this session's tasks always win over the
// disk copy on ID conflicts.
func (b *Board) mergeDisk() error {
	disk, err := Load(b.path)
	if err != nil {
		return err
	}
	known := make(map[string]bool, len(b.Tasks))
	for _, t := range b.Tasks {
		known[t.ID] = true
	}
	for _, t := range disk.Tasks {
		if _, gone := b.deleted[t.ID]; gone {
			continue // deleted this session: never merge back
		}
		if !known[t.ID] {
			b.Tasks = append(b.Tasks, t)
			b.dirty = true
		}
	}
	return nil
}

// Save merges concurrent writers and persists the board atomically: temp
// file first, then rename. It takes a brief exclusive lock, reloads the
// file, and pulls in any tasks written since this board was loaded — so an
// agent logging while the TUI is open (or two agents at once) never drops
// the other's cards. Tasks this session changed win over the disk copy;
// tasks only on disk are appended in disk order. Each writer still gets a
// unique temp file (via os.CreateTemp) so concurrent replacements never
// race over the same intermediate name. A clean board with nothing new on
// disk writes nothing, so read-only sessions stay read-only.
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
	return nil
}

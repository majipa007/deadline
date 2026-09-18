package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gotodo/internal/task"
)

// testRef pins headless timestamps the way fixedClock pins the TUI's.
var testRef = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)

func TestVersionPrintsBuildRevision(t *testing.T) {
	out := captureStdout(t, func() {
		if err := run([]string{"version"}); err != nil {
			t.Fatalf("version = %v, want nil", err)
		}
	})
	if strings.TrimSpace(out) != version {
		t.Errorf("version output = %q, want %q", out, version)
	}
}

// captureStdout runs fn with os.Stdout redirected to a pipe and returns
// what it printed. The installer relies on `gotodo version` printing the
// bare revision, so this pins the exact bytes.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	buf, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(buf)
}

func TestResolveID(t *testing.T) {
	b := &task.Board{Tasks: []task.Task{
		{ID: "abc123", Title: "[Alpha]"},
		{ID: "abc456", Title: "[Alpha beta]"},
		{ID: "def789", Title: "[Old]", Archived: true},
	}}
	if _, err := resolveID(b, "zzz"); err == nil {
		t.Error("resolveID(missing) = nil, want an error")
	}
	if _, err := resolveID(b, ""); err == nil {
		t.Error("resolveID(empty) = nil, want a usage error")
	}
	got, err := resolveID(b, "abc123")
	if err != nil {
		t.Fatalf("resolveID(full) = %v, want nil", err)
	}
	if got.Title != "[Alpha]" {
		t.Errorf("Title = %q, want [Alpha]", got.Title)
	}
	if _, err := resolveID(b, "abc"); err == nil {
		t.Error("resolveID(shared prefix) = nil, want an ambiguity error")
	} else if !strings.Contains(err.Error(), "abc123") || !strings.Contains(err.Error(), "abc456") {
		t.Errorf("ambiguity error = %q, want it to list both full IDs", err)
	}
	if _, err := resolveID(b, "def"); err == nil {
		t.Error("resolveID(archived-only match) = nil, want an archived error")
	} else if !strings.Contains(err.Error(), "archived") {
		t.Errorf("archived error = %q, want it to say archived", err)
	}
}

func TestHeadlessRoundTripThroughRun(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init = %v, want nil", err)
	}
	if err := run([]string{"add", "[Ship it]", "-desc", "d", "-status", "testing-review"}); err != nil {
		t.Fatalf("add = %v, want nil", err)
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].Status != task.StatusTestingReview {
		t.Fatalf("board = %+v, want one testing-review task", b.Tasks)
	}
	id := b.Tasks[0].ID
	if err := run([]string{"move", id[:8], "shipped"}); err != nil {
		t.Fatalf("move = %v, want nil", err)
	}
	if err := run([]string{"move", id, "nope"}); err == nil {
		t.Error("move(bad status) = nil, want an error")
	}
	if err := run([]string{"list", "-status", "shipped"}); err != nil {
		t.Fatalf("list = %v, want nil", err)
	}
	re, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.ByStatus(task.StatusShipped)) != 1 {
		t.Fatalf("shipped tasks = %d, want 1", len(re.ByStatus(task.StatusShipped)))
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}, {"init", "-h"}, {"list", "-h"}, {"add"}, {"add", "-h"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%q) = %v, want nil", argv, err)
		}
	}
}

func TestAddRejectsFlagTitle(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "-desc"}); err == nil {
		t.Error("add(-desc as title) = nil, want an error")
	}
}

func TestAddValidationErrorsWriteNothing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	bad := [][]string{
		{"add", "   "}, // blank title
		{"add", "[Bad date]", "-deadline", "yes"},        // garbage deadline
		{"add", "[Bad date]", "-deadline", "2026-08-04"}, // ISO, not DD/MM/YYYY
		{"add", "[Bad col]", "-status", "qa"},            // unknown column
		{"add", "[Extra]", "trailing"},                   // stray positional
	}
	// Note: bare `add` prints usage and exits zero by design (see
	// TestHelpExitsZero), so it is not an error case.
	for _, argv := range bad {
		if err := run(argv); err == nil {
			t.Errorf("run(%q) = nil, want an error", argv)
		}
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 0 {
		t.Errorf("tasks = %d after rejected adds, want 0", len(b.Tasks))
	}
}

func TestMoveErrorsAndNoOp(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "[Mover]"}); err != nil {
		t.Fatal(err)
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	id := b.Tasks[0].ID
	if err := run([]string{"move", "zzz", "indev"}); err == nil {
		t.Error("move(unknown id) = nil, want an error")
	}
	if err := run([]string{"move", id[:8]}); err == nil {
		t.Error("move(missing status) = nil, want a usage error")
	}
	// Same-status move is a no-op that still succeeds.
	out := captureStdout(t, func() {
		if err := run([]string{"move", id[:8], "todo"}); err != nil {
			t.Fatalf("move(same status) = %v, want nil", err)
		}
	})
	if !strings.Contains(out, "already in") {
		t.Errorf("move(same status) printed %q, want an already-in note", out)
	}
	re, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks[0].History) != 0 {
		t.Errorf("history = %+v after a no-op move, want none", re.Tasks[0].History)
	}
}

func TestListOutput(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := run([]string{"list"}); err != nil {
			t.Fatalf("list = %v, want nil", err)
		}
	})
	if strings.TrimSpace(out) != "no tasks" {
		t.Errorf("list(empty) = %q, want %q", out, "no tasks")
	}
	if err := run([]string{"add", "[Dated]", "-deadline", "04/08/2026", "-status", "blocked"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "[Plain]", "-status", "todo"}); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if err := run([]string{"list", "-status", "blocked"}); err != nil {
			t.Fatalf("list = %v, want nil", err)
		}
	})
	if !strings.Contains(out, "[blocked] [Dated]") || !strings.Contains(out, "due 04/08/2026") {
		t.Errorf("list(-status blocked) = %q, want the dated blocked card", out)
	}
	if strings.Contains(out, "[Plain]") {
		t.Errorf("list(-status blocked) = %q, want the todo card filtered out", out)
	}
	if err := run([]string{"list", "-status", "qa"}); err == nil {
		t.Error("list(bad status) = nil, want an error")
	}
}

func TestUnknownCommandErrors(t *testing.T) {
	if err := run([]string{"delete", "abc"}); err == nil {
		t.Error("run(delete) = nil, want an unknown-command error")
	} else if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run(delete) error = %q, want an unknown-command message", err)
	}
}

func TestAddKeepsLocalCalendarDate(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "[Dated]", "-deadline", "04/08/2026"}); err != nil {
		t.Fatalf("add = %v, want nil", err)
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 1 || b.Tasks[0].Deadline == nil {
		t.Fatalf("board = %+v, want one dated task", b.Tasks)
	}
	if got := b.Tasks[0].Deadline.Format(dateLayout); got != "04/08/2026" {
		t.Errorf("deadline = %q, want 04/08/2026 in the local zone", got)
	}
}

func TestInitCreatesProjectBoard(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init = %v, want nil", err)
	}
	b, err := task.Load(filepath.Join(dir, ".deadline", "board.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.DoneStatus(); got != task.StatusShipped {
		t.Errorf("DoneStatus() = %q, want shipped (init always creates dev)", got)
	}
	// Idempotent: a second init keeps the board instead of erroring.
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("second init = %v, want nil", err)
	}
	// A nested directory resolves to the same project board.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, filepath.Join(dir, "sub"))
	got, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".deadline", "board.json"); got != want {
		t.Errorf("resolveBoardPath() = %q, want %q", got, want)
	}
	// init inside the project reuses its board instead of nesting a new one.
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("nested init = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", ".deadline")); !os.IsNotExist(err) {
		t.Error("nested init created sub/.deadline, want the parent board reused")
	}
}

func TestEmptyShadowDirDoesNotClaimProject(t *testing.T) {
	outer := t.TempDir()
	chdir(t, outer)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	shadow := filepath.Join(outer, "sub", "empty")
	if err := os.MkdirAll(filepath.Join(shadow, ".deadline"), 0o755); err != nil {
		t.Fatal(err)
	}
	chdir(t, shadow)
	got, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(outer, ".deadline", "board.json"); got != want {
		t.Errorf("resolveBoardPath() = %q, want %q (empty .deadline skipped)", got, want)
	}
}

func TestInitGitignoresProjectBoard(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init = %v, want nil", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "bin/\n.deadline/\n"; string(data) != want {
		t.Errorf(".gitignore = %q, want %q", data, want)
	}
}

func TestInitCreatesGitignoreWhenMissing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatalf("init = %v, want nil", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf(".gitignore was not created: %v", err)
	}
	if string(data) != ".deadline/\n" {
		t.Errorf(".gitignore = %q, want %q", data, ".deadline/\n")
	}
}

func TestListSweepStaysInMemory(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	old := testRef.Add(-30 * 24 * time.Hour)
	id := b.Add("[Shipped long ago]", "", nil, old).ID
	for _, s := range []task.Status{task.StatusInDev, task.StatusTestingReview, task.StatusShipped} {
		if err := b.Move(id, s, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	// list hides the swept task but never writes: the file still holds it
	// unarchived for the next real session.
	if err := run([]string{"list"}); err != nil {
		t.Fatalf("list = %v, want nil", err)
	}
	re, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if re.Tasks[0].Archived {
		t.Error("list archived the task on disk, want read-only behaviour")
	}
	swept := *re
	if n := swept.SweepArchive(testRef); n != 1 {
		t.Errorf("SweepArchive = %d, want 1 on reload", n)
	}
}

func TestConcurrentAddsKeepBothTasks(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			title := "[Task one]"
			if i == 1 {
				title = "[Task two]"
			}
			errs[i] = run([]string{"add", title})
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("concurrent add = %v, want nil", err)
		}
	}
	path, err := resolveBoardPath()
	if err != nil {
		t.Fatal(err)
	}
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 2 {
		t.Errorf("tasks = %d, want 2 (no lost update)", len(b.Tasks))
	}
}

func TestSecondLockerFailsAfterTimeout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	hold, err := task.LockBoard(path)
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	start := time.Now()
	if _, err := task.LockBoard(path); err == nil {
		t.Error("second LockBoard = nil, want a busy error")
	} else if !strings.Contains(err.Error(), "busy") {
		t.Errorf("second LockBoard error = %q, want a busy message", err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("second lock failed after %v, want it to wait out the retries", elapsed)
	}
}

// chdir moves the test into dir and restores the working directory when
// the test finishes. (t.Chdir needs Go 1.24; the module targets 1.22.)
func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(prev); err != nil {
			t.Error(err)
		}
	})
}

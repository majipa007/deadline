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

// testRef pins headless timestamps.
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

// captureStdout returns what fn wrote to os.Stdout.
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

func TestBoardReportsTypeAndColumnsWithoutStoragePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "board.json")
	out := captureStdout(t, func() {
		if err := run([]string{"-file", path, "board"}); err != nil {
			t.Fatalf("board(personal) = %v", err)
		}
	})
	if !strings.Contains(out, "board: personal") || !strings.Contains(out, "columns: todo, doing, blocked, done") {
		t.Errorf("personal board output = %q", out)
	}
	if strings.Contains(out, path) || strings.Contains(out, ".json") {
		t.Errorf("board output leaks storage path and invites direct file access: %q", out)
	}

	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.SetColumns(task.DevColumns)
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if err := run([]string{"-file", path, "board"}); err != nil {
			t.Fatalf("board(dev) = %v", err)
		}
	})
	if !strings.Contains(out, "board: development") || !strings.Contains(out, "columns: todo, indev, testing-review, blocked, shipped") {
		t.Errorf("development board output = %q", out)
	}
	if err := run([]string{"-file", path, "board", "extra"}); err == nil {
		t.Error("board(extra) = nil, want a usage error")
	}
}

func TestHelpExitsZero(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}, {"init", "-h"}, {"board", "-h"}, {"list", "-h"}, {"show", "-h"}, {"add"}, {"add", "-h"}, {"edit"}, {"edit", "-h"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%q) = %v, want nil", argv, err)
		}
	}
}

func TestHeadlessParentSubtaskWorkflow(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "Parent"}); err != nil {
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
	parent := b.Tasks[0].ID
	if err := run([]string{"add", "Child", "-parent", parent[:8]}); err != nil {
		t.Fatalf("add child = %v", err)
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Tasks) != 2 || b.Tasks[1].ParentID != parent {
		t.Fatalf("tasks = %+v, want child linked to parent", b.Tasks)
	}
	child := b.Tasks[1].ID

	list := captureStdout(t, func() {
		if err := run([]string{"list", "-parent", parent[:8]}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(list, "Child") || !strings.Contains(list, "parent "+parent[:8]) {
		t.Errorf("parent-filtered list = %q", list)
	}
	show := captureStdout(t, func() {
		if err := run([]string{"show", parent[:8]}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(show, "subtasks: 0/1 complete") || !strings.Contains(show, "Child") {
		t.Errorf("show parent = %q", show)
	}

	if err := run([]string{"edit", child[:8], "-parent", ""}); err != nil {
		t.Fatalf("detach child = %v", err)
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := b.TaskByID(child)
	if !ok || got.ParentID != "" {
		t.Fatalf("detached child = %+v, found=%v", got, ok)
	}
}

func TestHeadlessRejectsNestedSubtasks(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "Parent"}); err != nil {
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
	parent := b.Tasks[0].ID
	if err := run([]string{"add", "Child", "-parent", parent[:8]}); err != nil {
		t.Fatal(err)
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	child := b.Tasks[1].ID
	if err := run([]string{"add", "Grandchild", "-parent", child[:8]}); err == nil {
		t.Fatal("nested add = nil, want an error")
	}
}

func TestEditUpdatesOnlySuppliedFieldsAndCanClearOptionalFields(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "Original", "-desc", "old detail", "-deadline", "04/08/2026"}); err != nil {
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
	id := b.Tasks[0].ID[:8]

	if err := run([]string{"edit", id, "-title", "Updated"}); err != nil {
		t.Fatalf("edit title = %v", err)
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Tasks[0]; got.Title != "Updated" || got.Description != "old detail" || got.Deadline == nil {
		t.Fatalf("partial edit = %+v, want preserved description and deadline", got)
	}

	if err := run([]string{"edit", id, "-desc", "", "-deadline", ""}); err != nil {
		t.Fatalf("clear optional fields = %v", err)
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Tasks[0]; got.Title != "Updated" || got.Description != "" || got.Deadline != nil {
		t.Fatalf("cleared edit = %+v", got)
	}
}

func TestEditValidationErrorsWriteNothing(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "Original", "-desc", "keep"}); err != nil {
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
	id := b.Tasks[0].ID[:8]
	bad := [][]string{
		{"edit", id},
		{"edit", id, "-title", "   "},
		{"edit", id, "-deadline", "2026-08-04"},
		{"edit", "missing", "-desc", "changed"},
	}
	for _, argv := range bad {
		if err := run(argv); err == nil {
			t.Errorf("run(%q) = nil, want an error", argv)
		}
	}
	b, err = task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Tasks[0]; got.Title != "Original" || got.Description != "keep" {
		t.Fatalf("task after rejected edits = %+v", got)
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
	// Bare `add` prints usage and exits zero, so it is not listed here.
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
	if err := run([]string{"frobnicate", "abc"}); err == nil {
		t.Error("run(frobnicate) = nil, want an unknown-command error")
	} else if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("run(frobnicate) error = %q, want an unknown-command message", err)
	}
}

func TestDeleteHeadless(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "[Drop]", "-status", "testing-review"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "[Keep]"}); err != nil {
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
	var drop, keep string
	for _, tc := range b.Tasks {
		if tc.Title == "[Drop]" {
			drop = tc.ID
		} else {
			keep = tc.ID
		}
	}
	out := captureStdout(t, func() {
		if err := run([]string{"delete", drop[:8]}); err != nil {
			t.Fatalf("delete = %v, want nil", err)
		}
	})
	if !strings.Contains(out, "deleted") || !strings.Contains(out, "[Drop]") {
		t.Errorf("delete printed %q, want a receipt naming the card", out)
	}
	re, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 1 || re.Tasks[0].ID != keep {
		t.Fatalf("tasks = %+v, want only the kept card", re.Tasks)
	}
	// Deleting it again fails.
	if err := run([]string{"delete", drop[:8]}); err == nil {
		t.Error("delete(gone id) = nil, want an error")
	}
	if err := run([]string{"delete"}); err == nil {
		t.Error("delete(no id) = nil, want a usage error")
	}
	if err := run([]string{"delete", keep[:8], "extra"}); err == nil {
		t.Error("delete(extra arg) = nil, want a usage error")
	}
}

func TestDeleteHeadlessRejectsAmbiguousAndArchived(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "board.json")
	b, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b.Add("[Alpha]", "", nil, testRef)
	b.Add("[Alpha beta]", "", nil, testRef)
	old := b.Add("[Old]", "", nil, testRef)
	if err := b.Move(old.ID, task.StatusDone, testRef); err != nil {
		t.Fatal(err)
	}
	// Pin IDs so the prefixes below are deterministic.
	b.Tasks[0].ID, b.Tasks[1].ID, b.Tasks[2].ID = "abc123", "abc456", "def789"
	b.Tasks[2].Archived = true
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	if err := runDelete(path, []string{"abc"}); err == nil {
		t.Error("delete(shared prefix) = nil, want an ambiguity error")
	}
	if err := runDelete(path, []string{"def"}); err == nil {
		t.Error("delete(archived-only match) = nil, want an archived error")
	}
	re, err := task.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(re.Tasks) != 3 {
		t.Errorf("tasks = %d after rejected deletes, want 3", len(re.Tasks))
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
	// list hides the swept task but never writes it back.
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

// chdir moves the test into dir until it finishes (t.Chdir needs Go 1.24).
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

// -worktree is set by add, printed by show and list, and cleared by edit.
func TestWorktreeFlagAddShowEdit(t *testing.T) {
	chdir(t, t.TempDir())
	if err := run([]string{"init"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"add", "Parser", "-worktree", "../wt-parser"}); err != nil {
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
	id := b.Tasks[0].ID[:8]
	if out := captureStdout(t, func() { run([]string{"show", id}) }); !strings.Contains(out, "worktree: ../wt-parser") {
		t.Errorf("show = %q", out)
	}
	if out := captureStdout(t, func() { run([]string{"list"}) }); !strings.Contains(out, "worktree ../wt-parser") {
		t.Errorf("list = %q", out)
	}
	if err := run([]string{"edit", id, "-worktree", ""}); err != nil {
		t.Fatal(err)
	}
	if b, _ = task.Load(path); b.Tasks[0].Worktree != "" {
		t.Errorf("edit -worktree \"\" left %q", b.Tasks[0].Worktree)
	}
}

// Command gotodo is a keyboard-driven terminal kanban board.
//
// Without a subcommand it opens the interactive board: the nearest
// .deadline/board.json walking up from the working directory (one board per
// project), or the global personal board outside any initialised project.
// With a subcommand it runs headless (no TUI) so scripts and coding agents
// can log work:
//
//	gotodo init                                              # one dev board in ./.deadline/
//	gotodo add "Rewrite the parser" -desc "..." -deadline 04/08/2026
//	gotodo add "Write parser tests" -parent 3fa1c9e2
//	gotodo board                                             # board type + accepted columns
//	gotodo list
//	gotodo list -status blocked
//	gotodo show 3fa1c9e2
//	gotodo move 3fa1c9e2 testing-review
//	gotodo edit 3fa1c9e2 -desc "Ready for review"
//	gotodo edit 3fa1c9e2 -worktree ../deadline-parser
//	gotodo delete 3fa1c9e2
//	gotodo version
//
// The global -file flag, when given, must precede the subcommand.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"gotodo/internal/task"
	"gotodo/internal/ui"
)

// dateLayout is the only accepted deadline shape, shared with the TUI form.
const dateLayout = "02/01/2006"

// shortLen is how much of a task ID headless output prints; commands accept
// a unique prefix.
const shortLen = 8

// projectDirName is the per-project board directory.
const projectDirName = ".deadline"

// projectBoardFile is the board file inside a project directory.
const projectBoardFile = "board.json"

// addUsage is printed for `add -h` and malformed add invocations.
const addUsage = `usage: gotodo add "title" [-desc ...] [-deadline DD/MM/YYYY] [-status ...] [-parent ID] [-worktree PATH]`

const editUsage = `usage: gotodo edit <id> [-title ...] [-desc ...] [-deadline DD/MM/YYYY] [-parent ID] [-worktree PATH]`

// version is the built revision, stamped via
//
//	go build -ldflags "-X main.version=<commit sha>"
//
// Unstamped builds report "dev". install.sh compares it to HEAD to skip rebuilds.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gotodo:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("gotodo", flag.ContinueOnError)
	file := fs.String("file", "", "path to the tasks JSON file (default: project board, else personal board)")
	// Parsing stops at the subcommand; each subcommand parses its own flags.
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args := fs.Args()

	// init runs before board resolution so it works where no board exists.
	if len(args) > 0 && args[0] == "init" {
		return runInit(args[1:])
	}

	path := *file
	if path == "" {
		var err error
		path, err = resolveBoardPath()
		if err != nil {
			return err
		}
	}

	if len(args) == 0 {
		return runTUI(path)
	}
	switch args[0] {
	case "add":
		return runAdd(path, args[1:])
	case "list":
		return runList(path, args[1:])
	case "show":
		return runShow(path, args[1:])
	case "board":
		return runBoard(path, args[1:])
	case "move":
		return runMove(path, args[1:])
	case "edit":
		return runEdit(path, args[1:])
	case "delete":
		return runDelete(path, args[1:])
	case "version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unknown command %q (want init, board, add, list, show, move, edit, delete or version)", args[0])
	}
}

// runBoard reports the resolved board type and status pipeline. It does not
// print the JSON path: agents should use the CLI, not the storage file.
func runBoard(path string, args []string) error {
	fs := flag.NewFlagSet("board", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo board)", fs.Arg(0))
	}
	b, err := task.Load(path)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(b.Statuses()))
	for _, s := range b.Statuses() {
		names = append(names, string(s))
	}
	kind := "personal"
	if b.DoneStatus() == task.StatusShipped {
		kind = "development"
	}
	fmt.Printf("board: %s\ncolumns: %s\n", kind, strings.Join(names, ", "))
	return nil
}

// resolveBoardPath returns the nearest project board walking up from the
// working directory, else the global personal board.
func resolveBoardPath() (string, error) {
	if board, err := findProjectBoard(); err != nil {
		return "", err
	} else if board != "" {
		return board, nil
	}
	return task.DefaultPath()
}

// findProjectBoard returns the nearest ancestor's .deadline/board.json, or
// "". A .deadline directory without a board file does not count.
func findProjectBoard() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("locate working directory: %w", err)
	}
	for {
		candidate := filepath.Join(dir, projectDirName, projectBoardFile)
		if st, err := os.Stat(candidate); err == nil {
			if st.IsDir() {
				return "", fmt.Errorf("%s is a directory, want a board file", candidate)
			}
			return candidate, nil
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("stat %s: %w", candidate, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// runInit creates ./.deadline/board.json with the dev columns and gitignores
// it. Inside an existing project it reuses that board instead of nesting.
func runInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (usage: gotodo init)", fs.Arg(0))
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("locate working directory: %w", err)
	}
	if board, err := findProjectBoard(); err != nil {
		return err
	} else if board != "" {
		fmt.Printf("already initialised: %s\n", board)
		return nil
	}
	path := filepath.Join(cwd, projectDirName, projectBoardFile)
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	if len(board.Columns) > 0 || len(board.Tasks) > 0 {
		fmt.Printf("already initialised: %s\n", path)
		return nil
	}
	board.SetColumns(task.DevColumns)
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("initialised %s [dev]\n", path)
	ensureGitignored(cwd)
	return nil
}

// ensureGitignored appends .deadline/ to the project's .gitignore, creating
// it if needed.
func ensureGitignored(dir string) {
	ignore := filepath.Join(dir, ".gitignore")
	data, err := os.ReadFile(ignore)
	if err != nil && !os.IsNotExist(err) {
		fmt.Println("hint: add .deadline/ to .gitignore to keep the board local")
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); t == ".deadline/" || t == ".deadline" ||
			t == "/.deadline/" || t == ".deadline/*" {
			return
		}
	}
	f, err := os.OpenFile(ignore, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Println("hint: add .deadline/ to .gitignore to keep the board local")
		return
	}
	defer f.Close()
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		fmt.Fprintln(f)
	}
	fmt.Fprintln(f, projectDirName+"/")
	fmt.Println("added .deadline/ to .gitignore")
}

// runTUI sweeps the archive, runs the interactive board, and saves on exit.
func runTUI(path string) error {
	board, err := task.Load(path)
	if err != nil {
		return err
	}

	board.SweepArchive(time.Now())

	p := tea.NewProgram(ui.NewApp(board), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	// Only save when dirty so a read-only session never writes.
	if board.Dirty() {
		if err := board.Save(); err != nil {
			return fmt.Errorf("save failed: %w", err)
		}
	}
	return nil
}

// runAdd appends one task headless. The title comes first; the rest parses
// as flags.
func runAdd(path string, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(addUsage)
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("bad title %q (%s)", args[0], addUsage)
	}
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	desc := fs.String("desc", "", "task description")
	deadlineStr := fs.String("deadline", "", "deadline as DD/MM/YYYY")
	statusStr := fs.String("status", "", "starting column")
	parentStr := fs.String("parent", "", "parent task ID prefix")
	worktree := fs.String("worktree", "", "git worktree path or branch for this task")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), addUsage)
	}
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())

	title := strings.TrimSpace(args[0])
	if title == "" {
		return errors.New("title must not be blank")
	}
	deadline, err := parseHeadlessDeadline(*deadlineStr)
	if err != nil {
		return err
	}
	status := board.Statuses()[0]
	if *statusStr != "" {
		s, err := boardStatus(board, *statusStr)
		if err != nil {
			return err
		}
		status = s
	}
	parentID := ""
	if *parentStr != "" {
		parent, err := resolveID(board, *parentStr)
		if err != nil {
			return fmt.Errorf("parent: %w", err)
		}
		parentID = parent.ID
	}
	now := time.Now()
	var t *task.Task
	if parentID == "" {
		t = board.Add(title, *desc, deadline, now)
	} else {
		t, err = board.AddChild(parentID, title, *desc, deadline, now)
		if err != nil {
			return err
		}
	}
	id, addedTitle := t.ID, t.Title
	if err := board.SetWorktree(id, *worktree, now); err != nil {
		return err
	}
	if status != board.Statuses()[0] {
		if err := board.Move(id, status, now); err != nil {
			return err
		}
	}
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("added %s [%s] %s\n", shortID(id), status, addedTitle)
	return nil
}

// parseHeadlessDeadline parses a DD/MM/YYYY flag at local midnight (like the
// TUI). Blank means no deadline, which edit uses to clear one.
func parseHeadlessDeadline(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	d, err := time.ParseInLocation(dateLayout, value, time.Local)
	if err != nil {
		return nil, fmt.Errorf("bad deadline %q (want DD/MM/YYYY)", value)
	}
	return &d, nil
}

// runList prints one line per active task. Read-only: the archive sweep is
// applied in memory and never saved.
func runList(path string, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	statusStr := fs.String("status", "", "only show one column")
	parentStr := fs.String("parent", "", "only show children of one task")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())
	now := time.Now()
	filter := task.Status("")
	if *statusStr != "" {
		s, err := boardStatus(board, *statusStr)
		if err != nil {
			return err
		}
		filter = s
	}
	parentID := ""
	if *parentStr != "" {
		parent, err := resolveID(board, *parentStr)
		if err != nil {
			return fmt.Errorf("parent: %w", err)
		}
		parentID = parent.ID
	}
	printed := 0
	for _, s := range board.Statuses() {
		if filter != "" && s != filter {
			continue
		}
		for _, t := range board.ByStatus(s) {
			if parentID != "" && t.ParentID != parentID {
				continue
			}
			line := fmt.Sprintf("%s [%s] %s", shortID(t.ID), s, t.Title)
			if t.Deadline != nil {
				line += " · due " + t.Deadline.In(now.Location()).Format(dateLayout)
			}
			if t.ParentID != "" {
				if parent, ok := board.TaskByID(t.ParentID); ok {
					line += " · parent " + shortID(parent.ID) + " " + parent.Title
				}
			} else if done, total := board.ChildProgress(t.ID); total > 0 {
				line += fmt.Sprintf(" · subtasks %d/%d", done, total)
			}
			if t.Worktree != "" {
				line += " · worktree " + t.Worktree
			}
			fmt.Println(line)
			printed++
		}
	}
	if printed == 0 {
		fmt.Println("no tasks")
	}
	return nil
}

// runShow prints one task with its relationship context and direct subtasks.
func runShow(path string, args []string) error {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		fmt.Println("usage: gotodo show <id>")
		return nil
	}
	if len(args) != 1 {
		return errors.New("usage: gotodo show <id>")
	}
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())
	t, err := resolveID(board, args[0])
	if err != nil {
		return err
	}
	fmt.Printf("%s [%s] %s\n", shortID(t.ID), t.Status, t.Title)
	if t.ParentID != "" {
		if parent, ok := board.TaskByID(t.ParentID); ok {
			fmt.Printf("parent: %s [%s] %s\n", shortID(parent.ID), parent.Status, parent.Title)
		}
	}
	if t.Deadline != nil {
		fmt.Printf("deadline: %s\n", t.Deadline.In(time.Local).Format(dateLayout))
	}
	if t.Worktree != "" {
		fmt.Printf("worktree: %s\n", t.Worktree)
	}
	if t.Description != "" {
		fmt.Printf("description:\n%s\n", t.Description)
	}
	children := board.Children(t.ID)
	if len(children) > 0 {
		done, total := board.ChildProgress(t.ID)
		fmt.Printf("subtasks: %d/%d complete\n", done, total)
		for _, child := range children {
			archived := ""
			if child.Archived {
				archived = " archived"
			}
			fmt.Printf("  %s [%s%s] %s\n", shortID(child.ID), child.Status, archived, child.Title)
		}
	}
	return nil
}

// runMove moves one task to another column: move <id-prefix> <status>.
func runMove(path string, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: gotodo move <id> <status>")
	}
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())

	t, err := resolveID(board, args[0])
	if err != nil {
		return err
	}
	to, err := boardStatus(board, args[1])
	if err != nil {
		return err
	}
	from := t.Status
	if from == to {
		// No-op move, but Save still merges anything written concurrently.
		if err := board.Save(); err != nil {
			return err
		}
		fmt.Printf("%s %s is already in %s\n", shortID(t.ID), t.Title, to)
		return nil
	}
	if err := board.Move(t.ID, to, time.Now()); err != nil {
		return err
	}
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("moved %s %s: %s → %s\n", shortID(t.ID), t.Title, from, to)
	return nil
}

// runEdit changes the supplied task fields headless; omitted flags keep their
// values, and an empty -desc, -deadline or -worktree clears it. Status
// changes go through move.
func runEdit(path string, args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Println(editUsage)
		return nil
	}
	if strings.HasPrefix(args[0], "-") {
		return fmt.Errorf("bad task id %q (%s)", args[0], editUsage)
	}
	fs := flag.NewFlagSet("edit", flag.ContinueOnError)
	titleFlag := fs.String("title", "", "replacement title")
	descFlag := fs.String("desc", "", "replacement description (blank clears)")
	deadlineFlag := fs.String("deadline", "", "replacement deadline as DD/MM/YYYY (blank clears)")
	parentFlag := fs.String("parent", "", "replacement parent ID prefix (blank detaches)")
	worktreeFlag := fs.String("worktree", "", "replacement worktree path or branch (blank clears)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q (%s)", fs.Arg(0), editUsage)
	}
	set := make(map[string]bool, 3)
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if len(set) == 0 {
		return errors.New(editUsage)
	}

	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())
	t, err := resolveID(board, args[0])
	if err != nil {
		return err
	}
	title, desc, deadline := t.Title, t.Description, t.Deadline
	parentID := t.ParentID
	if set["title"] {
		title = *titleFlag
	}
	if set["desc"] {
		desc = *descFlag
	}
	if set["deadline"] {
		deadline, err = parseHeadlessDeadline(*deadlineFlag)
		if err != nil {
			return err
		}
	}
	if set["parent"] {
		parentID = ""
		if *parentFlag != "" {
			parent, err := resolveID(board, *parentFlag)
			if err != nil {
				return fmt.Errorf("parent: %w", err)
			}
			parentID = parent.ID
		}
	}
	now := time.Now()
	if err := board.EditWithParent(t.ID, parentID, title, desc, deadline, now); err != nil {
		return err
	}
	if set["worktree"] {
		if err := board.SetWorktree(t.ID, *worktreeFlag, now); err != nil {
			return err
		}
	}
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("edited %s [%s] %s\n", shortID(t.ID), t.Status, strings.TrimSpace(title))
	return nil
}

// runDelete removes one active task headless: delete <id-prefix>. There is
// no confirm step; the printed receipt names what was removed.
func runDelete(path string, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: gotodo delete <id>")
	}
	board, err := task.Load(path)
	if err != nil {
		return err
	}
	board.SweepArchive(time.Now())

	t, err := resolveID(board, args[0])
	if err != nil {
		return err
	}
	detached := len(board.Children(t.ID))
	if err := board.Delete(t.ID); err != nil {
		return err
	}
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("deleted %s [%s] %s", shortID(t.ID), t.Status, t.Title)
	if detached > 0 {
		fmt.Printf(" · detached %d subtasks", detached)
	}
	fmt.Println()
	return nil
}

// resolveID finds the one active task whose ID starts with arg; a match on
// an archived task is reported as an error.
func resolveID(b *task.Board, arg string) (task.Task, error) {
	if arg == "" {
		return task.Task{}, errors.New("task id must not be blank")
	}
	var active, archived []task.Task
	for _, t := range b.Tasks {
		if strings.HasPrefix(t.ID, arg) {
			if t.Archived {
				archived = append(archived, t)
			} else {
				active = append(active, t)
			}
		}
	}
	switch len(active) {
	case 0:
		if len(archived) > 0 {
			return task.Task{}, fmt.Errorf("task %s is archived", shortID(archived[0].ID))
		}
		return task.Task{}, fmt.Errorf("no task id starts with %q", arg)
	case 1:
		return active[0], nil
	default:
		ids := make([]string, 0, len(active))
		for _, t := range active {
			ids = append(ids, t.ID)
		}
		return task.Task{}, fmt.Errorf("ambiguous id %q (%d tasks match: %s)", arg, len(active), strings.Join(ids, ", "))
	}
}

// boardStatus parses a status name against this board's own columns.
func boardStatus(b *task.Board, name string) (task.Status, error) {
	s := task.Status(name)
	if !b.HasStatus(s) {
		names := make([]string, 0, len(b.Statuses()))
		for _, c := range b.Statuses() {
			names = append(names, string(c))
		}
		return "", fmt.Errorf("unknown status %q for this board (want %s)", name, strings.Join(names, ", "))
	}
	return s, nil
}

func shortID(id string) string {
	if len(id) > shortLen {
		return id[:shortLen]
	}
	return id
}

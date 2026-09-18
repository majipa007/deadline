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
//	gotodo list
//	gotodo list -status blocked
//
// gotodo move 3fa1c9e2 testing-review
// gotodo version
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

// shortLen is how much of a task ID headless output prints: enough to
// move the task again, since move accepts a unique prefix.
const shortLen = 8

// projectDirName is the per-project board directory, kept gitignorable by
// design: one project never sees another project's board.
const projectDirName = ".deadline"

// projectBoardFile is the board file inside a project directory.
const projectBoardFile = "board.json"

// addUsage is printed for `add -h` and malformed add invocations.
const addUsage = `usage: gotodo add "title" [-desc ...] [-deadline DD/MM/YYYY] [-status ...]`

// version is the built revision, stamped at build time:
//
//	go build -ldflags "-X main.version=<commit sha>"
//
// Unstamped (e.g. local `go build`) reports "dev". The installer compares
// this against the cloned HEAD to skip rebuilds when already current.
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
	// The flag package stops at the first non-flag argument, which is the
	// subcommand; each subcommand parses its own flags from what remains.
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	args := fs.Args()

	// init targets ./.deadline unconditionally and runs before any board
	// resolution, so it works even where no board exists yet.
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
	case "move":
		return runMove(path, args[1:])
	case "version":
		fmt.Println(version)
		return nil
	default:
		return fmt.Errorf("unknown command %q (want init, add, list, move or version)", args[0])
	}
}

// resolveBoardPath finds the board for this invocation: the nearest
// .deadline/board.json walking up from the working directory, so every
// initialised project gets its own board; outside any project it is the
// global personal board.
func resolveBoardPath() (string, error) {
	if board, err := findProjectBoard(); err != nil {
		return "", err
	} else if board != "" {
		return board, nil
	}
	return task.DefaultPath()
}

// findProjectBoard walks up from the working directory to the nearest
// ancestor containing a .deadline/board.json file, or "" when there is
// none. Empty .deadline directories are skipped past: only a real board
// file claims the project.
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

// runInit creates ./.deadline/board.json with the dev columns. One project,
// one board: if this directory is already inside an initialised project,
// that board is reused instead of nesting a new one. (A nested board is
// still possible by hand-creating .deadline/board.json; the nearest file
// always wins.) It keeps the board out of version control via .gitignore.
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

// ensureGitignored keeps the project board out of version control: it
// appends .deadline/ to the project's .gitignore, creating the file when
// the project has none yet.
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

// runTUI is the interactive board: tidy, run, and save on change. Saves
// merge whatever agents wrote while the board was open.
func runTUI(path string) error {
	board, err := task.Load(path)
	if err != nil {
		return err
	}

	// Tidy the board before the first frame: anything in the terminal
	// column for two weeks belongs in the archive, not on the board.
	board.SweepArchive(time.Now())

	p := tea.NewProgram(ui.NewApp(board), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return err
	}
	// Final save covers any state the dirty path missed. Skipped when the
	// board isn't dirty so a read-only session can't clobber another
	// instance's save with a stale snapshot.
	if board.Dirty() {
		if err := board.Save(); err != nil {
			return fmt.Errorf("save failed: %w", err)
		}
	}
	return nil
}

// runAdd appends one task headless: add "title" [-desc ...]
// [-deadline DD/MM/YYYY] [-status ...]. The title comes first so the common
// case reads naturally; the rest parses as flags.
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
	var deadline *time.Time
	if *deadlineStr != "" {
		// Local midnight, like the TUI form: UTC midnight would shift the
		// date for timezones west of Greenwich.
		d, err := time.ParseInLocation(dateLayout, *deadlineStr, time.Local)
		if err != nil {
			return fmt.Errorf("bad deadline %q (want DD/MM/YYYY)", *deadlineStr)
		}
		deadline = &d
	}
	status := board.Statuses()[0]
	if *statusStr != "" {
		s, err := boardStatus(board, *statusStr)
		if err != nil {
			return err
		}
		status = s
	}
	now := time.Now()
	t := board.Add(title, *desc, deadline, now)
	if status != board.Statuses()[0] {
		if err := board.Move(t.ID, status, now); err != nil {
			return err
		}
	}
	if err := board.Save(); err != nil {
		return err
	}
	fmt.Printf("added %s [%s] %s\n", shortID(t.ID), status, t.Title)
	return nil
}

// runList prints one line per task: id, column, title, deadline when set.
// Archived tasks stay hidden, same as on the board. Read-only: the sweep
// below applies in memory only and is never saved.
func runList(path string, args []string) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	statusStr := fs.String("status", "", "only show one column")
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
	printed := 0
	for _, s := range board.Statuses() {
		if filter != "" && s != filter {
			continue
		}
		for _, t := range board.ByStatus(s) {
			line := fmt.Sprintf("%s [%s] %s", shortID(t.ID), s, t.Title)
			if t.Deadline != nil {
				line += " · due " + t.Deadline.In(now.Location()).Format(dateLayout)
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

// runMove slides one task to another column: move <id-prefix> <status>.
// The ID may be abbreviated as long as it still matches exactly one active
// task.
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

// resolveID finds the one active task whose ID starts with arg. Archived
// tasks are reported rather than silently moved while invisible.
func resolveID(b *task.Board, arg string) (task.Task, error) {
	if arg == "" {
		return task.Task{}, errors.New("usage: gotodo move <id> <status>")
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

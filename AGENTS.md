# deadline (gotodo)

Keyboard-driven terminal kanban board with Board, Analytics, Archive and Calendar tabs. Personal boards have four columns; project boards use a five-stage development pipeline. Go 1.22+, three deps (`bubbles`, `bubbletea`, `lipgloss`).

```sh
go test ./...   # full suite
go vet ./...    # strict, keep clean
gofmt -l .      # must print nothing
go build -o ~/.local/bin/gotodo .   # install the binary under test
```

<!-- deadline:begin - managed by the gotodo installer, do not edit between the markers -->
## GoTodo task board

You are working in a project tracked on a `gotodo` kanban board. For EVERY
code change — feature, fix, refactor — log it before starting and move it
along as you work:

1. `gotodo board` first. Never assume columns; use the pipeline it prints:
   `todo → doing → done` (personal) or
   `todo → indev → testing-review → shipped` (development). Either may use
   `blocked` when stuck.
2. `gotodo list` and skip a duplicate card if one already covers the work.
3. `gotodo add "title" [-desc "detail"] [-status todo]` before writing code.
4. Use `-parent <id-prefix>` when the work is a subtask of an existing card;
   inspect the family with `gotodo show <id-prefix>` or `list -parent`.
5. `gotodo move <id-prefix> <active-column>` when starting, then through the
   reported pipeline's review/terminal columns as work is verified and done.
6. `gotodo edit <id-prefix> [flags]` for title, description, deadline, parent, or worktree changes.
7. `gotodo delete <id-prefix>` only for cards logged by mistake — no undo.

Headless commands only (never run bare `gotodo`, it opens a TUI). If `gotodo`
is not on PATH, try exactly `~/.local/bin/gotodo`; do not search the filesystem
for it. If missing there too, tell the human to install it and continue without
tracking. Use the CLI only: never read, search, parse, or edit
`.deadline/board.json`, `tasks.json`, or any other board storage file directly.
<!-- deadline:end -->

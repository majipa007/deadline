---
name: gotodo-task-board
description: Task management and kanban board for coding agents via the gotodo CLI. Use for every software task, code change, feature, bug fix, refactor, implementation, test, or review: track work through todo, indev, testing-review, blocked, and shipped.
---

# GoTodo Task Board

Track dev work on a per-project kanban board with the `gotodo` CLI. Project and personal boards are resolved by the CLI; their persistence files are private implementation details.

## Workflow

1. **Inspect the board:** `gotodo board` prints the board type and accepted columns. Never guess the pipeline. Personal boards use `todo → doing → done`; development boards use `todo → indev → testing-review → shipped`; both may use `blocked`.
2. **Look first:** `gotodo list` and avoid duplicating a card that already exists. Run `gotodo init` only when a project-local development board is wanted and none exists.
3. **Log before starting:** `gotodo add "title" [-desc "detail"] [-deadline DD/MM/YYYY] [-status todo]` — it prints the id; keep its short prefix for later moves.
4. **Group related work:** when work belongs under an existing parent task, use `gotodo add "title" -parent <parent-id>`. Inspect a family with `gotodo show <parent-id>` or `gotodo list -parent <parent-id>`. Keep hierarchy one level deep.
5. **Move it along:** `gotodo move <id-prefix> <status>` through the exact columns reported by `gotodo board`. Any unique id prefix works. A parent cannot finish before all subtasks.
6. **Update details through the CLI:** `gotodo edit <id-prefix> [-title "..."] [-desc "..."] [-deadline DD/MM/YYYY] [-parent <id>] [-worktree PATH]`. Pass `-worktree` (also on `add`) when the work lives in a git worktree. An explicitly blank description, deadline, parent, or worktree clears it; use `move` for status.
7. **Stuck:** move to `blocked` and record the reason with `edit -desc`; move back to the reported active column (`doing` or `indev`) when unblocked.
8. **Wrong card:** `gotodo delete <id-prefix>` removes it headless — no confirm or undo. Deleting a parent detaches its subtasks; archived cards cannot be deleted headless.

## Rules

- One card per work item; `list` first, then `add` only when it is really new.
- Headless commands only — bare `gotodo` opens the interactive TUI, never run that here.
- CLI only: never read, search, parse, or edit `.deadline/board.json`, `tasks.json`, or any board storage file directly. Use `gotodo board`, `list`, `show`, `add`, `move`, `edit`, and `delete`.
- If `gotodo` is not on PATH, try exactly `~/.local/bin/gotodo`. Do not search the filesystem for it. If that is also missing, tell the human to install it and continue without tracking.

---
name: deadline
description: gotodo deadline board — project kanban (todo, indev, testing-review, blocked, shipped) via the gotodo CLI. Use when implementing a feature, fixing a bug, or starting, progressing, or finishing any dev work item: log it on the board first.
---

# Deadline — project task board

Track dev work on a per-project kanban board with the `gotodo` CLI. Each project has its own board in `./.deadline/` (gitignored, invisible to other projects). The human's personal board lives elsewhere — never touch files outside the current project.

## Workflow

1. **Look first:** `gotodo list` to see the board and avoid duplicating a card that already exists. If there is no board yet (`.deadline/` missing), run `gotodo init` once — it creates `./.deadline/board.json` with the dev pipeline. Never `init` over an existing board.
2. **Log before starting:** `gotodo add "title" [-desc "detail"] [-deadline DD/MM/YYYY] [-status todo]` — it prints the id; keep its short prefix for later moves.
3. **Move it along:** `gotodo move <id-prefix> <status>` as work progresses: `todo → indev → testing-review → shipped`. Any unique id prefix works. The human sees your writes live in their open board.
4. **Stuck:** `gotodo move <id> blocked` and record the reason in the description; `gotodo move <id> indev` when unblocked.

Columns: `todo, indev, testing-review, blocked, shipped` — pass `testing-review` literally.

## Rules

- One card per work item; `list` first, then `add` only when it is really new.
- Headless commands only — bare `gotodo` opens the interactive TUI, never run that here.
- There is no delete/remove command — only the human removes cards in the TUI. Never invent `gotodo delete`; it errors as an unknown command.
- If `gotodo` is not on PATH, stop and tell the human to install it instead of working around it.

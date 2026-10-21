<div align="center">

<img src="banner.png" alt="deadline" width="100%">

*Green. Amber. Red. Then a cross.*

![Go](https://img.shields.io/badge/go-1.22-00ADD8?logo=go&logoColor=white)
![Stars](https://img.shields.io/github/stars/19Naveen/deadline?style=flat)
![Last commit](https://img.shields.io/github/last-commit/19Naveen/deadline)

A terminal kanban board that never lets you forget when something is due.

</div>

---

## The board

Four columns. Two-line cards. A countdown on each card that gets louder as the day closes in.

```
  Board    Analytics    Archive    Calendar
╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮╭──────────────────────╮
│ TODO (3)             ││ DOING (1)            ││ BLOCKED (1)          ││ DONE (1)             │
│                      ││                      ││                      ││                      │
│ ▌ Ship the Q3 re… 6d ││   Rewrite the pa… 2d ││   Waiting on de… 12d ││   Drop the old API   │
│ ▌ Draft, review, se… ││   Split the lexer o… ││   Mocks not landed   ││                      │
│                      ││                      ││                      ││                      │
│   Renew the TLS … 1d ││                      ││                      ││                      │
│   Staging box        ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│   Chase the… late 3d ││                      ││                      ││                      │
│   Third follow-up    ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
│                      ││                      ││                      ││                      │
╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯╰──────────────────────╯

hjkl Move  enter Open  a Add  s Subtask  e Edit  d Delete  m Grab  f Family  ctrl+t Focus
tab Switch  ? Help  q Quit
```

No mouse. No config file. No account.

---

## Deadlines

Each card with a deadline ends its title row
with a countdown, and it only gets louder as the day gets closer:

| Time left | Colour | Looks like |
|---|---|---|
| more than 3 days | green | `9d` |
| 3 days or less | amber, bold | `3d` |
| due today or tomorrow | red, bold | `today`, `1d` |
| the day has passed | red, bold | `late 3d` |
| task is done | no countdown | |

The date itself, in DD/MM/YYYY, is in the detail view and on the Calendar
page. A finished task never turns red: being late is history by then, not an
alarm. The selected card, focused column and active tab are marked in blue.

Deadlines are optional. Leave the field blank and the card has no countdown.

---

## Analytics

Second page. Everything is derived from each task's transition history, so it is measured rather than tallied.

```
╭────────╮╭─────────╮╭───────────╮╭────────╮
│    3   ││    1    ││     1     ││    1   │
│  TODO  ││  DOING  ││  BLOCKED  ││  DONE  │
╰────────╯╰─────────╯╰───────────╯╰────────╯
THROUGHPUT
             █
1 completed over 14 days · 18/07/2026 → 31/07/2026
CYCLE TIME
mean 3d 22h · median 3d 22h · over 1 completed
mean time spent per column:
todo     ████████████████████████ 3d 22h
doing    ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
blocked  ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
done     ░░░░░░░░░░░░░░░░░░░░░░░░ 0m
BLOCKED
1 currently blocked
  Waiting on design                        2h
STREAK
current 1 day · longest 1 day
          May 2026                      June 2026                      July 2026
 Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su
                  1   2   3      1   2   3   4   5   6   7              1   2   3   4   5
  4   5   6   7   8   9  10      8   9  10  11  12  13  14      6   7   8   9  10  11  12
 11  12  13  14  15  16  17     15  16  17  18 ▄19 ▄20  21     13  14  15  16  17  18  19
 18  19  20 ▄21  22  23  24     22  23  24  25  26  27  28    ▄20 ▄21  22  23  24  25  26
 25  26  27  28  29  30  31     29  30                        █27 ▄28 ▄29  30  31
▁▄▓█ more finished that day · underline is today, 30/07/2026
```

Cycle time runs from created to done. Time-per-column shows where work actually sits, which is rarely where you think.

The streak is up to six real months, not an anonymous strip: a day you finished something is shaded, darker the more you closed, and today is underlined. Narrow the terminal and it shows fewer months, down to one.

---

## Archive

Third page. A task that has sat in Done for 14 days moves here on its own, at launch and once an hour while running. The board stays short without you pruning it.

It is read-only. Nothing you can press there will change a task.

Archived tasks still count in throughput, cycle time and the streak. Hiding a task never erases it from your history — only the four column tiles and the board itself stop counting it.

---

## Calendar

Fourth page. Every open deadline on the board, on real months, in the colour it already wears on its card.

```
DEADLINES
         July 2026                     August 2026                   September 2026
 Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su     Mo  Tu  We  Th  Fr  Sa  Su
          1   2   3   4   5                          1   2          1   2   3   4   5   6
  6   7   8   9  10  11  12      3 ● 4   5   6   7   8   9      7   8   9  10  11  12 ●13
 13  14  15  16  17  18  19     10  11 ●12  13  14  15  16     14  15  16  17  18  19  20
 20  21  22  23  24  25  26     17  18  19  20  21  22  23     21  22  23  24  25  26  27
●27  28  29 ●30 ●31             24  25  26 ●27  28  29  30     28  29  30
● due · green >3 days · amber ≤3 · red ≤1 or overdue · underline is today
UPCOMING
● 27/07/2026  Chase the invoice                     3 days late
● 30/07/2026  Renew the TLS cert                    today
● 04/08/2026  Ship the report                       in 5 days
```

`h` and `l` step a month either way, `t` comes back to this one. A day with more than one deadline takes the colour of its most pressing task.

Done tasks are left off: a deadline you already met is history, and this page is about what is coming. Each month needs about 31 columns, so a wide terminal shows up to six side by side; 90 columns fit three, and below 59 just one. The upcoming list fills whatever rows are left and says how many it could not fit.

---

## Install

Needs Go 1.22 or newer, and a terminal at least 80 columns wide.

**Quick install** (Debian, Ubuntu, Arch, Fedora, openSUSE, Alpine — sets up `~/.bashrc` and `~/.zshrc` automatically):

```bash
curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash
```

The script detects your distro (`apt`, `pacman`, `dnf`/`yum`, `zypper`, `apk`), installs `git` and Go ≥ 1.22 if missing (falling back to an official Go toolchain in `~/.local/go` when the distro package is too old or root is unavailable), builds `gotodo` into `~/.local/bin`, and adds it to `PATH` in both `~/.bashrc` and `~/.zshrc` without duplicating lines. It also installs the `deadline` agent skill for Claude Code, Codex, OpenCode and other Agent-Skills-compatible tools, so coding agents can log work to project boards. Restart your terminal afterwards, then run `gotodo`.

**Manual install:**

```bash
git clone https://github.com/19Naveen/deadline.git
cd deadline
go build -trimpath -ldflags "-s -w" -o ~/.local/bin/gotodo .
```

**Upgrade:** re-run the installer — it rebuilds only when `main` moved past your installed copy (checked via `gotodo version`), otherwise it just re-syncs the skill and PATH lines.

**Uninstall:**

```bash
curl -fsSL https://raw.githubusercontent.com/19Naveen/deadline/main/install.sh | bash -s -- --uninstall
```

This removes the binary, the agent skills and the PATH lines it added. Boards are data, not installation, so `~/.config/gotodo` and `.deadline/` in projects are kept — delete them too for a full wipe.

Then run it:

```bash
gotodo
```

If you get `gotodo: command not found`, `~/.local/bin` is not on your `PATH`:

```bash
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc   # or ~/.bashrc
```

---

## Keys

| Key | What it does |
|---|---|
| `enter` | expand the selected task in a centred popup — `e` edit, `d` delete, `esc` close |
| `a` | add a task |
| `s` | add a subtask under the selected task's family |
| `f` | focus the board on the selected parent and its subtasks; press again to clear |
| `e` | edit the selected task |
| `d` | delete it, after a `y`/`n` confirm popup that names the task |
| `m` | grab it, then `h`/`l` to drag between columns, `enter` to drop, `esc` to cancel |
| `h` `l` | previous / next column |
| `j` `k` | previous / next task |
| `g` `G` | first / last task in the column |
| `ctrl+t` | switch between moving the cursor and moving between whole columns |
| `tab` | cycle Board, Analytics, Archive, Calendar |
| `h` `l` `t` | on the Calendar page: previous month, next month, back to today |
| `?` | key list |
| `q` | quit |

`enter` on a card opens it centred on the screen: the full title, its
parent, deadline, worktree and ID, then the subtask checklist and the
description. Long bodies scroll with `j`/`k`; short ones stay compact. It is
read-only, but `s`, `e` and `d` work from there.

Adding and editing open the same form, centred in the same place. `tab` and
`shift+tab` move between Title, Description, Deadline, Worktree and Parent.
Worktree is optional free text — the git worktree path or branch the work
lives in — and a new subtask starts with its parent's.
On Parent, type a title or ID to filter the visible dropdown, use `up`/`down`
to highlight a match, and press `enter` to choose it. Clear the text for no
parent. The dropdown shows titles and short IDs together, so nothing must be
memorized. After choosing, `enter` saves; `esc` throws the form away.

The Description is multi-line: `ctrl+j` starts a new line, and `enter` still
saves. The editor opens 15 rows tall, shrinking only when the terminal
cannot fit it.

Parent cards show a progress bar (`▰▱ 1/2 subtasks`). Subtasks keep their own
column and deadline, show `↳ parent` on the card, and appear in the parent's
detail checklist. Selecting any card marks the rest of its family with a dim
bar, across columns. A parent cannot enter the terminal column until every
subtask is complete. Deleting a parent safely detaches rather than deletes its
subtasks.

Dates go in as `DD/MM/YYYY`. A date it cannot read keeps the form open and tells you the format, rather than quietly dropping what you typed.

Or do not type them at all. Tab to the Deadline field and a calendar is
already there. `hjkl` moves a day or a week and fills the field as you go,
`t` jumps back to today. Today is green; the selected day is bracketed.

The calendar and the text box share the keyboard rather than fighting over
it — `hjkl` steer the calendar because those letters are never part of a
date, while digits and `backspace` go to the field and the calendar follows
along. `tab` and `enter` behave exactly as they do on the other two fields.

---

## Storage

Two boards, never mixed. Outside any project, `gotodo` opens your personal board, one JSON file at `~/.config/gotodo/tasks.json`. Inside an initialised project (any directory at or under one containing `.deadline/board.json`), it opens that project's board instead — one project never sees another project's tasks. Point anywhere else with `-file`:

```bash
gotodo -file ./work.json
```

Every change writes through a temporary file and a rename, so an interrupted write cannot leave you with half a board. A session where you changed nothing does not write at all. Concurrent writers merge by task under a brief file lock, so an agent logging while your board is open never drops your cards — and vice versa; two sessions editing the same card resolve last-writer-wins.

---

## Project boards

A project board is the dev pipeline: `TODO → IN DEV → TESTING/REVIEW → BLOCKED → SHIPPED`, with shipped playing the role done plays on the personal board (grey deadlines, throughput, streak, and the 14-day archive sweep). The personal board keeps the original four columns. Five columns need a wider terminal — about 100 columns instead of 80.

Create one per project:

```bash
cd my-project
gotodo init
```

This writes `.deadline/board.json` and adds `.deadline/` to the project's `.gitignore`, creating the file when the project has none yet. Running `init` twice, or inside a subdirectory, reuses the existing board instead of nesting a new one. (A nested board is still possible by hand-creating `.deadline/board.json`; the nearest file always wins.)

Separate files are separate boards, which is the easiest way to keep work and personal apart — and project boards are how agents log development in parallel with your own list (see below).

---

## Agents

Coding agents can't drive the interactive board, so `gotodo` has headless commands with plain-text output. They run against the project board when invoked in the project directory:

```bash
gotodo init                                    # once per project (skip if .deadline/ exists)
gotodo board                                   # board type + accepted columns
gotodo add "Rewrite the parser" -desc "Split the lexer" -deadline 04/08/2026
gotodo add "Write parser tests" -parent 3fa1c9e2
gotodo list
gotodo list -status blocked
gotodo list -parent 3fa1c9e2
gotodo show 3fa1c9e2                         # details, parent and subtasks
gotodo move 3fa1c9e2 testing-review          # any unique id prefix works
gotodo edit 3fa1c9e2 -parent 91bc2f0a         # attach; -parent "" detaches
gotodo edit 3fa1c9e2 -worktree ../app-parser   # where the work lives; "" clears
gotodo delete 3fa1c9e2                       # no confirm; list first, there is no undo
```

The global `-file` flag must precede the subcommand (`gotodo -file ./work.json list`). A bare `gotodo -h`, or `init`, `add` or `list` with `-h`, prints help without changing anything.

The installer drops a `gotodo-task-board` skill (`skills/gotodo-task-board/SKILL.md` in this repo) into the Claude, Codex, OpenCode and shared agent skills directories. Its task-management and kanban keywords make it easy for agents to discover for any coding workflow. An open board picks up agent writes within a couple of seconds — no restart needed.

---

## Development

```bash
go test ./...
go vet ./...
gofmt -l .
```

Three packages. `internal/task` is the model and the JSON store, `internal/stats` is pure analytics, and `internal/ui` contains the four Bubble Tea tabs: Board, Analytics, Archive and Calendar. Nothing in `internal/task` or `internal/stats` calls `time.Now` — the clock is always passed in, which is what makes the tests deterministic.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and [Lip Gloss](https://github.com/charmbracelet/lipgloss). Three dependencies, no more.

---

## FAQ

**Can I get a task back out of the archive?**
Not from inside the app. Edit the JSON and drop the `"archived": true` line.

**Why 14 days?**
Long enough that a finished task is still there when someone asks about it. Short enough that Done does not become a scrapbook.

**Does it sync?**
No. It is one file. Put it in a synced folder if you want it on two machines. Two sessions open at once merge by task instead of overwriting each other, with the later save winning any card both touched.

**Why does it say my terminal is too narrow?**
Four columns and a date need 80 columns (about 100 for a five-column project board). Below that it tells you, instead of drawing a board that overlaps itself. Press `tab` for Analytics, Archive and Calendar, which stay readable on narrower terminals.

**Do old task files still work?**
Yes. Files written before descriptions and deadlines existed load fine, with those fields empty.

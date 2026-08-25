<img src="docs/images/logo.png" alt="taskii logo" width="72">

# taskii — Obsidian PARA edition

A fast, keyboard-driven terminal dashboard for people who run their work out of an
Obsidian vault. Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea).

This is a fork of [parsaenami/taskii](https://github.com/parsaenami/taskii), which
provides the dashboard, Pomodoro timer, notes board and reports. The fork adds an
Obsidian vault index, Jira integration, deadlines and reminders, a calendar, and
phone notifications.

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go)
![License](https://img.shields.io/badge/license-MIT-blue)

Everything is stored locally. Nothing leaves the machine unless you switch on phone
notifications and give it a topic.

---

## Contents

- [What it does](#what-it-does)
- [Install](#install)
- [Quick start](#quick-start)
- [Concepts](#concepts)
- [The three views](#the-three-views)
- [Typing syntax](#typing-syntax)
- [Keybindings](#keybindings)
- [Obsidian integration](#obsidian-integration)
- [Jira integration](#jira-integration)
- [Calendar export](#calendar-export)
- [Notifications](#notifications)
- [Settings](#settings)
- [Command line](#command-line)
- [Where things are stored](#where-things-are-stored)
- [Development](#development)

---

## What it does

- **Reads your vault directly.** Tickets, projects, areas and their task lines are
  parsed off disk, so browsing is instant and keeps working with Obsidian closed.
- **Runs Jira actions** — fetch, status transitions, comments, worklogs — through
  the official Obsidian CLI and the `jira-sync` plugin.
- **Deadlines and start reminders** on tasks and subtasks, written into the vault
  in the conventions Obsidian plugins already read.
- **A calendar** with week, month and year views, and repeating events.
- **Today's timeline** on the dashboard, updating as you edit.
- **Push notifications** to your phone via ntfy, with a delivery log.
- **Writes local tasks and daily notes back into the vault**, opt-in.

Both integrations are optional. With Jira off it is a vault-backed task manager;
with Obsidian sync off it only reads; with both off it is the original taskii.

---

## Install

Requires [Go](https://go.dev) 1.26+ to build.

```bash
git clone https://github.com/Frank2533/taskii.git
cd taskii
go build -o ~/.local/bin/taskii .
```

Make sure `~/.local/bin` is on your `PATH`.

### For the Obsidian and Jira features

1. **Enable the Obsidian CLI** — in Obsidian: *Settings → General → Advanced*. This
   registers an `obsidian` binary into `~/.local/bin`. Only writes need it; reading
   the vault never does.
2. **Install the Obsidian plugins you want to drive**: `jira-sync` for Jira, and
   optionally [Full Calendar Remastered](https://github.com/obsidian-full-calendar-remastered/plugin-full-calendar)
   or [ICS Calendar Viewer](https://community.obsidian.md/plugins/ics-calendar-viewer)
   to see the exported calendar inside Obsidian.

---

## Quick start

```bash
taskii
```

It detects whichever vault Obsidian currently has open. Then:

- `?` — every keybinding for the current view
- `,` — settings
- `1` `2` `3` — dashboard, PARA, calendar

Add your first task with `a`. Type part of a Jira ticket summary and press `↓` to
attach it to that ticket; press Enter without selecting to add a plain task.

---

## Concepts

Three things look similar and behave differently:

| | What it is | Where it lives |
|---|---|---|
| **Task** | work to finish; can be ticked off | taskii's store, mirrored to a vault note if sync is on |
| **Ticket** | a Jira issue mirrored into your vault | a note in `Tickets/`, owned by `jira-sync` |
| **Event** | time that is spoken for, done or not | taskii's store, exported to `.ics` |

**A deadline is not the day a task sits under.** `Date` decides which list a task
appears in; `Due` is when it must be finished. A task can be worked on today and
due on Friday. Overdue therefore means two things — carried over from an earlier
day, or past its deadline — and both are shown, deadline misses first.

**Tickets are chosen, not mirrored.** Your dashboard is a working set you pick
from the typeahead, not a feed of everything in the vault.

**The vault is the source of truth for anything in it.** Ticking a ticket's subtask
in the dashboard writes into the note, so the two views cannot drift apart.

---

## The three views

### 1 — Dashboard

Today, Overdue, Notes, Reports, Pomodoro, and **Today's Timeline**: an hour scale
with a now marker showing appointments, start reminders and deadlines. It takes its
height from the Notes board and grows with how busy the day is, capped at half the
space the two share. Items with no clock time are counted separately rather than
pinned to an invented hour.

A ticket row expands to show its subtasks, foldable with `z`; a folded row still
reports its count.

### 2 — PARA

A navigator (areas, projects, local tasks), a ticket list, and a detail pane with
the note's task lines.

The **Unfiled** row is worth knowing about: your vault only archives a ticket when
it is closed *and* has an area set, so a closed ticket with no area is stranded in
`Tickets/` and invisible everywhere else. This row surfaces those; `u` files one.

### 3 — Calendar

Week, month and year grids. A day cell shows titles, an open-subtask count in
brackets, and `+N more` for what will not fit. The year view answers "when is it
busy" with per-month counts, since no cell at that zoom can hold text. On a narrow
terminal it falls back to a stacked agenda rather than shaving titles to initials.

---

## Typing syntax

Modifiers are typed inline, following taskii's existing habit where a trailing
`14:30` already turns a task into an appointment.

### Deadlines and reminders — while adding or editing a task

```
fix the zepto spider !today @3h
normalize cities !2d
check replication !tmr
review the PR !fri
ship it !2026-09-15
```

| Token | Meaning |
|---|---|
| `!today` `!tod` `!0d` | due end of today |
| `!tmr` `!tomorrow` | due end of tomorrow |
| `!2d` `!10days` | due in N days |
| `!mon`…`!sun` | due the coming weekday |
| `!w` `!eow` | due end of this week |
| `!2026-09-15` | due that date |
| `@3h` `@90m` | remind me to start, measured from now |

Anything unrecognised stays in the title — `email !bob about @home` is untouched.

Prefer not to type? `D` opens a picker: today, tomorrow, in N days, end of week,
clear, a one-hour reminder, or `c` for a custom offset in days, hours and minutes.

### Events — `a` in the calendar view

```
standup 09:30-09:45 weekdays
gym 07:00-08:00 mon,wed,fri
review 14:00-15:30 !fri
planning 10:00-11:00 monthly x2
deploy 23:00-01:00
```

| Token | Meaning |
|---|---|
| `09:30-10:00` | start and end — **required** |
| `!tmr` `!fri` `!2026-09-15` | which day (today if omitted) |
| `daily` `weekly` `monthly` `yearly` | repeat |
| `weekdays` `weekends` | Mon–Fri / Sat–Sun (implies weekly) |
| `mon,wed,fri` | specific days (implies weekly) |
| `x2` | every other period |

An end before the start runs past midnight. Without a time range you are describing
a task, not an event, and it is refused.

Note the difference: a bare `fri` names a day an event **recurs on**; `!fri` means
the coming Friday.

---

## Keybindings

`?` lists everything for the current view. The essentials:

### Dashboard

| Key | Action |
|---|---|
| `a` / `A` | add a task / add a subtask to the selected ticket |
| `N` | add a note about the task or subtask under the cursor |
| `e` | edit the task or subtask under the cursor |
| `space` `enter` | toggle done |
| `D` | deadline and reminder picker (`c` for a custom offset) |
| `d` / `i` | delete / mark important |
| `z` | fold a ticket's subtasks |
| `I` / `U` | filter important / unfinished |
| `tab` `↑↓ jk` | move |
| `p` `r` `n` | pomodoro start-pause, reset, skip |

### PARA

| Key | Action |
|---|---|
| `a` / `n` | add a task / a dated note to the selected ticket |
| `u` | file a stranded ticket into the next area |
| `o` / `r` | open the note in Obsidian / reindex |
| `R` `s` `c` `w` | Jira: fetch, status, comment, log tracked time |
| `p` | track pomodoro time against this ticket |

### Calendar

| Key | Action |
|---|---|
| `w` `m` `y` | week, month, year |
| `←→ hl` / `↑↓ kj` | move a day / a week |
| `[` `]` / `T` | previous, next period / today |
| `tab` | step through the day's entries |
| `a` `e` `d` | add, edit, delete an event |
| `C` | export the calendar now |

### Anywhere

`1` `2` `3` views · `,` settings · `?` all keys · `t` theme · `L` layout · `q` quit

---

## Obsidian integration

### Reading

Always on, needs nothing installed. taskii parses the vault directly:

- `Tickets/` and `Archive/**` — Jira issues, with their frontmatter and task lines
- `Projects/` — PARA projects, joined to tickets by `jira_epic` ↔ `epic_link`
- `Areas/*/Info.md` — areas
- any note carrying a `taskii_id` — a local task, wherever it sits

Reads deliberately bypass the Obsidian CLI: it costs a process spawn per call and
needs the app running, whereas reading files works with Obsidian closed.

### Writing — opt-in

Turn on **Write to Obsidian** in settings. taskii then:

- writes each local task to a note in your project folder, keyed by `taskii_id`, so
  renaming a task moves its note instead of orphaning it
- moves a note to `Archive/Local Tasks/` when the task is completed, and marks it
  `status: deleted` and archives it when the task is deleted
- writes the notes board to `Resources/YYYY-MM-DD.md`, rewritten so deletions
  propagate and skipped entirely when unchanged
- appends quick-added tasks and dated notes to a ticket's freeform sections

It never writes to the `jira-sync-section-*` blocks: those are rewritten wholesale
on every fetch, so anything put there is lost at the next sync.

taskii moves its own notes rather than relying on Obsidian's note-mover plugins,
whose rules are typically scoped to the tickets folder and would never see them.

### Notes on tasks

`N` adds a note about whatever the cursor is on, and the Notes pane follows the
selection while a task list has focus, returning to the day's board when focus
moves away. Where the note lands depends on what it is about:

| Selection | Written to |
|---|---|
| a subtask | an indented block directly under that task line |
| a ticket | the ticket's `## Work Log / Updates`, dated |
| a local task | the body of that task's own note, dated |

A note about one subtask belongs attached to it rather than in a section shared
by the whole note, which is why the three differ.

### Task line syntax

A subtask's schedule is written into its own line:

```markdown
- [ ] check zepto spider 📅 2026-08-27 (@2026-08-26 09:00)
```

`📅` is the Tasks plugin's due date; `(@…)` is the Reminder plugin's reminder,
which carries a time of day a due date cannot. Both are read back, so a deadline
set in taskii is a real deadline in the vault.

---

## Jira integration

Toggle it in settings. It drives the **`jira-sync`** Obsidian plugin through the
official Obsidian CLI, so Obsidian must be running — the plugin's logic lives
inside the app.

| Key | Plugin command |
|---|---|
| `R` | `batch-fetch-issues-jira` |
| `s` | `update-issue-status-jira` |
| `c` | `add-comment-jira` |
| `w` | `update-work-log-jira-batch` (opt-in) |

**Status is never written to frontmatter directly.** Doing so would rename the file,
trip the archive automation and push the result, all while Jira still held the old
value. Transitions go through the plugin.

Before dispatching, taskii asks the command whether it can run and reports why not.
The most common cause of everything being unavailable at once is `jira-sync` having
an active connection with no URL — the plugin checks that first, and a blocked
command disappears from the palette rather than explaining itself.

### Time tracking

`p` binds the Pomodoro to the selected ticket. Time accrues in taskii's own store,
outside the vault. `w` writes it to the note's work log. Sending it on to Jira is a
separate opt-in setting, and the local write always happens first so a failed API
call cannot lose your record.

---

## Calendar export

taskii writes an `.ics` containing ticket due dates, project targets, appointments
and events.

```bash
taskii ics -vault ~/Obsidian/MyVault -out ~/Obsidian/MyVault/Calendar/taskii.ics
```

The TUI also exports periodically in the background; `C` forces it.

Output is deterministic — sorted events, a `DTSTAMP` taken from the source note
rather than the clock, and identical content is never rewritten. This matters when
the file lives in a vault that is watched and auto-committed: output that varied
per run would produce an endless stream of no-op commits and cross-machine
conflicts.

Repeating events export as a single entry with an `RRULE`, and changed occurrences
as `EXDATE`, so a subscriber keeps the series intact.

The `ics` subcommand never calls the Obsidian CLI, deliberately: it is meant for a
timer, and the CLI launches Obsidian when it is not running.

---

## Notifications

Reminders announce on the desktop, and optionally on your phone.

Two things produce reminders, and both announce the same way — desktop, and
phone if it is switched on:

- **Tasks** remind once, at the moment you set with `@3h`, the picker's
  one-hour option, or a custom offset. Several coming due together are
  summarised into a single notification rather than a burst.
- **Subtasks** remind the same way. Their reminder lives in the task line in
  the vault, so it is swept from there — which also means one written directly
  in Obsidian is honoured.
- **Events remind three times**, at 15, 5 and 1 minutes before each occurrence.

A reminder that is
already late is dropped rather than delivered — a "15 minutes before" warning
arriving three minutes before is wrong about the one thing it exists to say — and
recorded as skipped so you can tell that apart from a failure.

### Phone push via ntfy

Enable it in settings and set a topic. Install the [ntfy](https://ntfy.sh) app and
subscribe to the same topic.

> **An ntfy topic is a shared channel, not an account.** On the public server anyone
> who knows or guesses your topic can read everything sent to it. Use a long random
> name, or self-host and point the server setting at it. taskii masks the topic on
> screen and keeps it out of logs and error messages for this reason.

### Delivery log

```bash
taskii notifications        # last 20
taskii notifications -n 0   # everything
```

Records `sent`, `failed` (with the reason), `skipped` (with why it was not
attempted), and `desktop` when push is off. ntfy's own server-side cache only holds
around 12 hours, so this is the durable record.

---

## Settings

`,` opens the settings screen.

| Setting | Default | Notes |
|---|---|---|
| Timezone | your system zone | IANA name; drives every date boundary and the exported calendar |
| Vault path | the vault Obsidian has open | |
| Calendar file | `<vault>/Calendar/taskii.ics` | |
| Export every | 15m | `0` disables the background exporter |
| Send worklog to Jira | off | on also pushes tracked time to the issue |
| Jira integration | on | off hides every Jira action |
| Write to Obsidian | **off** | on lets taskii create and move notes |
| Local task folder | `Projects` | finished ones move to `Archive/Local Tasks` |
| Phone notifications | **off** | the only thing that leaves this machine |
| ntfy topic / server | unset / `ntfy.sh` | |
| Theme, layout | | also cyclable with `t` and `L` |

Timezone is validated as you type it, not silently ignored at save time. Nothing is
hardcoded to any one zone.

---

## Command line

```
taskii                        # the dashboard
taskii --simple               # one combined list
taskii --mock                 # sample data; touches nothing real
taskii --vault PATH           # index a specific vault
taskii --data-dir PATH        # store taskii's own JSON elsewhere

taskii ics [-vault …] [-out …] [-tz …] [-quiet]
taskii notifications [-n N]
```

---

## Where things are stored

taskii's own data lives under `$XDG_DATA_HOME/taskii` (usually
`~/.local/share/taskii`):

| File | Contents |
|---|---|
| `tasks.json` | tasks |
| `notes.json` | the notes board |
| `events.json` | calendar events |
| `worklog.json` | tracked time, per Jira key |
| `reminders.json` | which reminders have fired |
| `notifications.log` | delivery history |
| `settings.json` | settings |

Nothing taskii owns is written into your vault unless Obsidian sync is on.

---

## Development

```bash
go build ./...
go vet ./...
go test ./...
```

The suite is hermetic: vault tests build fixtures in a temp directory, and nothing
touches a real vault, a real Jira, or a real notification service. Two opt-in tests
exercise live systems:

```bash
TASKII_TEST_VAULT=~/Obsidian/MyVault go test ./internal/para/ -run TestRealVault -v
TASKII_LIVE_OBSIDIAN=1 go test ./internal/obsidian/ -run TestLiveCheck -v
```

`AGENTS.md` is the decision log — why things are built the way they are, and which
failure modes the odder-looking choices exist to prevent. Worth reading before
changing anything that touches the vault, the calendar output, or reminders.

### Layout

| Package | Responsibility |
|---|---|
| `internal/para` | reads a PARA vault off disk |
| `internal/vault` | line-scoped, atomic note edits |
| `internal/obsidian` | drives the Obsidian CLI |
| `internal/model` | tasks, events, settings, storage |
| `internal/deadline` | deadline and reminder token parsing |
| `internal/ics`, `internal/export` | calendar rendering |
| `internal/remind`, `internal/notify` | reminder scheduling and delivery |
| `internal/worklog` | time tracking |
| `internal/ui` | Bubble Tea models and views |

---

## Credits

Built on [parsaenami/taskii](https://github.com/parsaenami/taskii) by Parsa Enami.

## License

MIT, as upstream.

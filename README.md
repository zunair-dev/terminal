# terminal

A single-file, Tokyo-Night–themed **terminal dashboard** for running a multi-service dev
stack. It tails each service's log file, shows live status (UP / STARTING / DOWN / SKIP),
aggregates errors & warnings into one pane, and lets you fire ad-hoc shell commands per
service — all inside a keyboard-driven TUI built on [`tview`](https://github.com/rivo/tview)
and [`tcell`](https://github.com/gdamore/tcell).

It was originally the dev dashboard for a Rails + Next.js + Expo monorepo, so the default
service list reflects that. It's a couple hundred lines of Go in a single `main.go` — read
it, tweak the `services` slice, and make it yours.

## Features

- **Per-service log panes** with syntax-aware colorizing (errors red, warnings amber,
  "ready/compiled/listening" green, request lines cyan).
- **All tab** — a 2×2 grid showing every service at once.
- **Errors tab** — a merged stream of every error/warning/fail/timeout/crash line across
  all services, tagged by origin.
- **Live status bar** — port-listen check (`lsof`) + PID liveness, refreshed every 3s.
- **Command mode** (`:`) — run an arbitrary shell command in a service's working directory
  and stream its output into that pane.
- **Mobile shortcuts** — `i` boots the iOS Simulator and opens the Expo dev URL; `a` does
  the same on an Android emulator via `adb`.

## Expected layout

The dashboard is a **viewer**, not a supervisor — it assumes something else (a `start.sh`,
`foreman`, etc.) launches the services and writes:

```
<root>/
├── tmp/logs/<name>.log   # one log file per service, appended live
└── tmp/pids/<name>.pid   # PID file per service (for liveness)
```

`<root>` is auto-detected (the parent of the binary, or the dir containing a `backend/`
folder), or you can pass it explicitly: `./terminal /path/to/project`.

## Configure your services

Edit the `services` slice at the top of `main.go`:

```go
var services = []Service{
    // Name       Label         Tech          Port  Log             Color                             Dir
    {"backend",  "Backend",    "Rails",      3000, "backend.log",  tcell.NewRGBColor(122, 162, 247), "backend"},
    {"jobs",     "Jobs",       "SolidQueue",    0, "jobs.log",     tcell.NewRGBColor(187, 154, 247), "backend"},
    {"frontend", "Frontend",   "Next.js",    3001, "frontend.log", tcell.NewRGBColor(158, 206, 106), "frontend"},
    {"app",      "Mobile App", "Expo",       8081, "app.log",      tcell.NewRGBColor(224, 175, 104), "app"},
}
```

`Port: 0` means "no port to check — use the PID file for liveness" (e.g. a worker process).
`Dir` is the service's working directory relative to `<root>`, used for command mode.

## Build & run

```bash
go build -o terminal .
./terminal              # auto-detect project root
./terminal /path/to/project
```

## Keybindings

| Key            | Action                                   |
| -------------- | ---------------------------------------- |
| `1`            | All (grid view)                          |
| `2`–`5`        | Jump to a service pane                   |
| `6`            | Errors & warnings pane                   |
| `Tab`          | Cycle tabs                               |
| `↑ / ↓`        | Scroll one line                          |
| `PgUp / PgDn`  | Scroll a page                            |
| `Home / End`   | Jump to top / bottom                     |
| `i` / `a`      | Open Expo app on iOS / Android           |
| `:`            | Command mode (run a shell command)       |
| `q` / `Ctrl-C` | Quit                                     |

## License

[MIT](./LICENSE)

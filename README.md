# Terminal

**One screen to watch your whole dev stack.** A single-file Go terminal dashboard that tails every service's logs, colorizes them, funnels all errors into one place, and shows you at a glance what's UP, STARTING, DOWN, or not running — without you juggling six terminal tabs.

Built with [`tview`](https://github.com/rivo/tview) + [`tcell`](https://github.com/gdamore/tcell) and themed after Tokyo Night.

```
 ◆ Zunair  dev dashboard                                   ⏱ 4m12s
  ◆ All   Backend Rails ●:3000  Jobs SolidQueue ●  Frontend Next.js ●:3001  Mobile App Expo ◐  ⚠ Errors
 ┌ Backend ───────────────────┐┌ Frontend ──────────────────┐
 │ Listening on port 3000     ││ ✓ Compiled /app in 812ms   │
 │ Started GET "/health"      ││ ○ compiling /dashboard...  │
 │ Completed 200 OK in 8ms    ││ ready in 1.4s              │
 └────────────────────────────┘└────────────────────────────┘
 ┌ Jobs ──────────────────────┐┌ Mobile App ────────────────┐
 │ [SolidQueue] worker ready  ││ Starting Metro Bundler...  │
 │ Performed EmailJob (12ms)  ││ › Press i to open iOS      │
 └────────────────────────────┘└────────────────────────────┘
 1all 2back 3jobs 4front 5app 6errors  tab cycle  ↑↓scroll  i iOS  a android  :cmd  q quit
```

> **The one thing to understand:** this is a **viewer, not a supervisor.** It never starts, stops, or restarts anything. You (or a start script, `Procfile`, `foreman`, `docker`, `tmux`…) launch the services and point their logs + PIDs at two folders; this tool just watches. That's the whole contract, and it's covered in [Using it in a real project](#using-it-in-a-real-project).

---

## Quick start

```bash
# 1. Build it
go build -o terminal .

# 2. Make sure your services write logs + pids where the tool looks:
#    <root>/tmp/logs/<name>.log   and   <root>/tmp/pids/<name>.pid
#    (see the launcher example below — copy/paste it)

# 3. Watch everything
./terminal                    # auto-detect the project root
./terminal /path/to/project   # …or point it somewhere explicitly
```

Then press `1`–`6` to switch tabs, `6` to see all errors in one stream, and `q` to quit.

## Requirements

| | |
| --- | --- |
| **Go** | 1.25.6 (see `go.mod`) |
| **OS** | macOS or Linux |
| **`lsof`** | must be on your `PATH` (used for the port-listen check) |

## How it works

The dashboard reads two things per service, on a loop — it never touches the processes themselves:

- **Logs** — tails `<root>/tmp/logs/<name>.log`, polling ~every **300ms** and printing only the new bytes since last read.
- **Liveness** — refreshed every **3s**: reads the PID from `<root>/tmp/pids/<name>.pid` and pings it with `kill -0`, plus a port check via `lsof -i :<port> -sTCP:LISTEN -t`.

That produces one of four statuses per service:

| Badge | Status | What it means |
| :---: | --- | --- |
| ● green | **UP** | Port is listening (or, for `Port: 0` services, the process is alive) |
| ◐ amber | **STARTING** | Process is alive but the port isn't listening *yet* |
| ● red | **DOWN** | A pid file exists but the process behind it is dead |
| ○ grey | **SKIP** | No pid file — nothing was ever launched under this name |

Log lines are colorized by content so problems jump out:

| Color | Triggered by |
| --- | --- |
| Red | `error`, `exception`, `fatal` |
| Amber | `warn` |
| Green | `started`, `listening`, `ready`, `compiled` |
| Cyan | `200`, `processing` (request-ish lines) |

`<root>` is auto-detected: it starts from the directory of the binary (walking up one level for `go run` / build-cache paths), and if that directory doesn't contain a `backend/` folder it walks up one more level to find it. Pass a path as the first argument to override detection entirely.

## Using it in a real project

This is the part that matters. Because the dashboard only **reads** logs and pid files, your launcher's job is to **write** them. The contract is two lines per service:

1. Send **stdout + stderr** to `<root>/tmp/logs/<name>.log`
2. Write the **PID** to `<root>/tmp/pids/<name>.pid`

…where `<name>` matches the `Name` in the [`services` slice](#configuring-your-own-services) (defaults: `backend`, `jobs`, `frontend`, `app`).

Here's a complete launcher you can drop into any project. It starts four services in the background, wires up their logs and pids, then runs the dashboard in the foreground so you watch them all. Swap in your own commands and directories.

```bash
#!/usr/bin/env bash
set -euo pipefail

# Run from your project root
ROOT="$(cd "$(dirname "$0")" && pwd)"
mkdir -p "$ROOT/tmp/logs" "$ROOT/tmp/pids"

# start <name> <dir> -- <command...>
start() {
  local name="$1" dir="$2"; shift 3   # drop name, dir, and the "--"
  ( cd "$ROOT/$dir" && exec "$@" ) \
    > "$ROOT/tmp/logs/$name.log" 2>&1 &
  echo $! > "$ROOT/tmp/pids/$name.pid"
  echo "started $name (pid $!)"
}

start backend   backend  -- bin/rails server -p 3000   # web service with a port
start jobs       backend  -- bin/jobs                    # background worker, no port -> Port: 0
start frontend   frontend -- npm run dev                 # web frontend with a port
start app        app      -- npx expo start              # mobile dev server

# Watch everything in one screen (foreground)
exec "$ROOT/terminal" "$ROOT"
```

Save it as `dev.sh`, run `chmod +x dev.sh`, then `./dev.sh`. Every service now streams into its own pane.

**The payoff:** when something breaks, you don't grep four terminals. Hit `6` for the **Errors** tab — every `error` / `warning` / `fail` / `timeout` / `crash` line from *all* services is merged into a single feed, each line tagged with the service it came from. One glance tells you who fell over and why.

When you're done, stop everything using the pids you recorded:

```bash
kill $(cat tmp/pids/*.pid) 2>/dev/null || true
```

## Configuring your own services

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

| Field | What it does |
| --- | --- |
| **`Name`** | The service's key. Sets the files the tool watches: `tmp/logs/<Name>.log` and `tmp/pids/<Name>.pid`. **Must match what your launcher writes.** |
| **`Label`** | Human-readable name shown in tabs and pane titles. |
| **`Tech`** | Short tech tag shown in the status bar (cosmetic). |
| **`Port`** | TCP port to check for a listener. Use **`0`** for portless services (workers, jobs) — liveness then relies on the PID alone. |
| **`Log`** | Log filename inside `tmp/logs/` (usually `<Name>.log`). |
| **`Color`** | Accent color for the pane border, title, and tab. |
| **`Dir`** | Working directory relative to `<root>`, used as the cwd for [command mode](#command-mode). |

> **Heads up:** the **All** tab is a fixed 2×2 grid of the first four services. If you add or remove services, tweak the grid layout in `main()` to match.

## Keybindings

Number keys map to tabs left-to-right: `1` = All, then one per service, then Errors last.

| Key | Action |
| --- | --- |
| `1` | All — 2×2 grid of every service |
| `2` | Backend |
| `3` | Jobs |
| `4` | Frontend |
| `5` | Mobile App |
| `6` | Errors — merged errors & warnings |
| `Tab` | Cycle to the next tab |
| `↑` / `↓` | Scroll one line |
| `PgUp` / `PgDn` | Scroll one page |
| `Home` / `End` | Jump to top / bottom |
| `i` | Open the app on the iOS Simulator *(Expo, optional)* |
| `a` | Open the app on an Android emulator *(Expo, optional)* |
| `:` | [Command mode](#command-mode) |
| `q` / `Ctrl-C` | Quit |

Scrolling acts on the current pane. The **All** grid has no single scroll target, so switch to an individual service or the Errors tab to scroll.

## Command mode

Press `:` on a service tab, type any shell command, and hit Enter. It runs with its cwd set to that service's `Dir` (relative to `<root>`), and its combined output streams live into the pane. Handy for a quick `bundle install`, `npm run lint`, or `git status` without leaving the dashboard. Press Esc to cancel. (If you're on **All** or **Errors** when you press `:`, it jumps to the first service first.)

## Mobile shortcuts (optional)

If one of your services is an Expo app, `i` opens it on the iOS Simulator and `a` on a connected Android emulator — both by opening `exp://127.0.0.1:8081` (via `npx uri-scheme` / `xcrun simctl` and `adb` respectively). If your stack has no mobile app, ignore these keys.

## Troubleshooting

| Symptom | Cause & fix |
| --- | --- |
| Service shows **`SKIP`** | No pid file at `tmp/pids/<name>.pid`. Your launcher didn't write one, or used a different name. Ensure `<name>` matches the `Name` in the slice and that you `echo $! > tmp/pids/<name>.pid`. |
| Stuck on **`STARTING`** | Process is alive but the port never opened. Confirm the service actually binds the `Port` you configured, that `lsof` is installed, and the port number matches reality. (Portless workers should use `Port: 0`.) |
| **No logs appear** | Either the log filename doesn't match the `Log` field, or `<root>` was mis-detected. Pass the root explicitly: `./terminal /path/to/project`. |

## License

[MIT](./LICENSE)

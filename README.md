# terminal

A single-file Go terminal dashboard (built on [`tview`](https://github.com/rivo/tview) + [`tcell`](https://github.com/gdamore/tcell), Tokyo Night theme) for watching a multi-service dev stack in one screen. It is a **passive viewer, not a process supervisor**: it never starts, stops, or restarts anything. Something else — a start script, `Procfile`, `foreman`, `docker`, `tmux`, etc. — launches your services; this tool just tails their logs, colorizes them, merges errors into one pane, and reports per-service status (UP / STARTING / DOWN / SKIP) by watching pid files and listening ports.

## Requirements

- **Go 1.25.6** (see `go.mod`)
- **macOS or Linux**
- **`lsof`** on your `PATH` (used for the port-listen check)

## Build & run

```bash
go build -o terminal .
./terminal                    # auto-detect project root
./terminal /path/to/project   # or point it at a root explicitly
```

`<root>` is auto-detected: it starts from the directory of the binary (walking up one level for `go run`/build-cache paths), and if that directory doesn't contain a `backend/` folder it walks up one more level to find it. You can always override detection by passing a path as the first CLI argument. From `<root>`, the tool reads:

```
<root>/tmp/logs/<name>.log   # one appended-to log file per service
<root>/tmp/pids/<name>.pid   # one pid file per service (contains the PID)
```

where `<name>` matches the `Name` field of each entry in the `services` slice.

## How it works

For each configured service, on a loop:

- **Logs** — tails `<root>/tmp/logs/<name>.log`, polling ~every **300ms** and appending only the new bytes since the last read.
- **Liveness** — refreshed every **3s**: reads the PID from `<root>/tmp/pids/<name>.pid` and checks it with `kill -0`, plus a port-listen check via `lsof -i :<port> -sTCP:LISTEN -t`.

Status per service:

| Status | Meaning |
| --- | --- |
| **UP** | Port is listening (or, for `Port: 0` services, the PID is alive) |
| **STARTING** | PID is alive but the port isn't listening yet |
| **DOWN** | A pid file exists but the process is dead |
| **SKIP** | No pid file present |

Log lines are colorized by content: **errors/exception/fatal** in red, **warnings** in amber, **started/listening/ready/compiled** in green, and **request-ish lines** (`200`, `processing`) in cyan.

## Using it in a real project with many services running

This is the important part. Because the dashboard only *reads* logs and pid files, your job is to make your launcher write them. The contract is:

1. Redirect each service's **stdout + stderr** to `<root>/tmp/logs/<name>.log`.
2. Write each service's **PID** to `<root>/tmp/pids/<name>.pid`.

…where `<name>` matches the `Name` in the `services` slice (defaults: `backend`, `jobs`, `frontend`, `app`).

Here's a complete, copy-pasteable launcher that starts four services in the background, wires up their log and pid files, and then runs the dashboard in the foreground so you can watch them all. Adapt the service commands and directories to your own stack.

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

# A web backend with a port (e.g. Rails)
start backend  backend  -- bin/rails server -p 3000
# A background worker with NO port (liveness is PID-only -> Port: 0)
start jobs      backend  -- bin/jobs
# A web frontend with a port (e.g. Next.js)
start frontend  frontend -- npm run dev
# A mobile dev server (e.g. Expo)
start app       app      -- npx expo start

# Watch everything in one screen (foreground)
exec "$ROOT/terminal" "$ROOT"
```

Save it as `dev.sh`, `chmod +x dev.sh`, and run `./dev.sh`. Each service's output now streams into its own pane. When one of them blows up, you don't have to hunt through four separate terminals — open the **Errors** tab (key `6`) and every error/warning/fail/timeout/crash line from *all* services is merged into a single stream, each line tagged with the service it came from. That's usually the fastest way to spot which service failed and why.

To stop everything, kill the PIDs you recorded, e.g.:

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

Each field:

- **`Name`** — the service's key. Determines the files the tool watches: `tmp/logs/<Name>.log` and `tmp/pids/<Name>.pid`. This must match what your launcher writes.
- **`Label`** — the human-readable name shown in tabs and pane titles.
- **`Tech`** — a short tech tag shown in the status bar (purely cosmetic).
- **`Port`** — the TCP port to check for listening. Use **`0`** for services with no port (e.g. background workers); liveness then falls back to the PID file alone.
- **`Log`** — the log filename inside `tmp/logs/` (typically `<Name>.log`).
- **`Color`** — the accent color for the pane border, title, and tab.
- **`Dir`** — the service's working directory relative to `<root>`, used as the cwd for command mode.

> Note: the **All** tab renders a fixed 2×2 grid of the first four services. If you change the number of services, adjust the grid layout in `main()` accordingly.

## Keybindings

Number keys map to tabs in the order they appear (`1` = All, then one per service, then Errors last):

| Key | Action |
| --- | --- |
| `1` | All (2×2 grid of every service) |
| `2` | Backend |
| `3` | Jobs |
| `4` | Frontend |
| `5` | Mobile App |
| `6` | Errors (merged errors & warnings) |
| `Tab` | Cycle to the next tab |
| `↑` / `↓` | Scroll one line |
| `PgUp` / `PgDn` | Scroll one page |
| `Home` / `End` | Jump to top / bottom |
| `i` | Open the app on the iOS Simulator *(Expo-specific, optional)* |
| `a` | Open the app on an Android emulator *(Expo-specific, optional)* |
| `:` | Command mode — run a shell command in the selected service's `Dir` |
| `q` / `Ctrl-C` | Quit |

Scrolling keys act on the currently selected pane; the **All** grid has no single scroll target, so switch to an individual service or the Errors tab to scroll.

The `i` and `a` shortcuts are mobile-only conveniences for an Expo `app` service (they use `npx uri-scheme` / `xcrun simctl` and `adb` to open `exp://127.0.0.1:8081`). Ignore them if your stack has no mobile app.

## Command mode

Press `:` while a service tab is selected (if you're on **All** or **Errors** it jumps to the first service), type a shell command, and press Enter. The command runs with its cwd set to that service's `Dir` (relative to `<root>`), and its combined stdout/stderr streams live into the pane. Press Esc to cancel.

## Troubleshooting

- **Service shows `SKIP`** — no pid file at `tmp/pids/<name>.pid`. Your launcher didn't write one (or wrote it under a different name). Make sure `<name>` matches the `Name` in the `services` slice and that you `echo $! > tmp/pids/<name>.pid`.
- **Stuck on `STARTING`** — the PID is alive but the port never came up. Confirm the service actually binds the `Port` you configured, that `lsof` is installed, and that the port number in the slice matches reality. (For portless workers, set `Port: 0`.)
- **No logs appear** — either the log filename doesn't match the `Log` field, or `<root>` was detected incorrectly (so the tool is looking in the wrong `tmp/logs`). Pass the root explicitly: `./terminal /path/to/project`.

## License

[MIT](./LICENSE)

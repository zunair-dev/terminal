package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// ── Service definitions ─────────────────────────────────────────────────────

type Service struct {
	Name  string
	Label string
	Tech  string
	Port  int // 0 = no port check
	Log   string
	Color tcell.Color
	Dir   string // relative to root
}

var services = []Service{
	{"backend", "Backend", "Rails", 3000, "backend.log", tcell.NewRGBColor(122, 162, 247), "backend"},
	{"jobs", "Jobs", "SolidQueue", 0, "jobs.log", tcell.NewRGBColor(187, 154, 247), "backend"},
	{"frontend", "Frontend", "Next.js", 3001, "frontend.log", tcell.NewRGBColor(158, 206, 106), "frontend"},
	{"app", "Mobile App", "Expo", 8081, "app.log", tcell.NewRGBColor(224, 175, 104), "app"},
}

// Tab indices: 0 = All, 1..N = individual services, last = Errors
const allTabIndex = 0

func svcTabIndex(svcIdx int) int { return svcIdx + 1 }
func errorsTabIndex() int        { return len(services) + 1 }
func tabCount() int              { return len(services) + 2 } // All + services + Errors

type Status int

const (
	StatusStarting Status = iota
	StatusUp
	StatusDown
	StatusSkipped
)

func (s Status) String() string {
	switch s {
	case StatusUp:
		return "UP"
	case StatusStarting:
		return "STARTING"
	case StatusDown:
		return "DOWN"
	case StatusSkipped:
		return "SKIP"
	}
	return "?"
}

func (s Status) Icon() string {
	switch s {
	case StatusUp:
		return "●"
	case StatusStarting:
		return "◐"
	case StatusDown:
		return "●"
	case StatusSkipped:
		return "○"
	}
	return "?"
}

func (s Status) Color() tcell.Color {
	switch s {
	case StatusUp:
		return tcell.NewRGBColor(158, 206, 106)
	case StatusStarting:
		return tcell.NewRGBColor(224, 175, 104)
	case StatusDown:
		return tcell.NewRGBColor(247, 118, 142)
	case StatusSkipped:
		return tcell.NewRGBColor(86, 95, 137)
	}
	return tcell.ColorWhite
}

// ── Colors ──────────────────────────────────────────────────────────────────

var (
	colorBg      = tcell.NewRGBColor(26, 27, 38)
	colorSurface = tcell.NewRGBColor(36, 40, 59)
	colorBorder  = tcell.NewRGBColor(65, 72, 104)
	colorActive  = tcell.NewRGBColor(122, 162, 247)
	colorText    = tcell.NewRGBColor(192, 202, 245)
	colorDim     = tcell.NewRGBColor(86, 95, 137)
	colorCyan    = tcell.NewRGBColor(125, 207, 255)
	colorGreen   = tcell.NewRGBColor(158, 206, 106)
	colorRed     = tcell.NewRGBColor(247, 118, 142)
	colorYellow  = tcell.NewRGBColor(224, 175, 104)
)

// ── State ───────────────────────────────────────────────────────────────────

var (
	root      string
	logDir    string
	pidDir    string
	startTime = time.Now()
	mu        sync.Mutex
	statuses  = make(map[string]Status)
	filePos   = make(map[string]int64)
	activeTab = 0 // 0 = All (default)
	cmdMode   = false
)

// ── Helpers ─────────────────────────────────────────────────────────────────

func elapsed() string {
	d := time.Since(startTime)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func checkPort(port int) bool {
	if port == 0 {
		return false
	}
	out, err := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-sTCP:LISTEN", "-t").Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func pidAlive(name string) bool {
	data, err := os.ReadFile(filepath.Join(pidDir, name+".pid"))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}
	return exec.Command("kill", "-0", strconv.Itoa(pid)).Run() == nil
}

func readNewLines(name, logFile string) string {
	path := filepath.Join(logDir, logFile)
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	size := info.Size()
	pos := filePos[name]

	if size == pos {
		return ""
	}
	if size < pos {
		pos = 0
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	f.Seek(pos, io.SeekStart)
	data, err := io.ReadAll(f)
	if err != nil {
		return ""
	}
	filePos[name] = size
	return string(data)
}

func updateStatuses() {
	mu.Lock()
	defer mu.Unlock()
	for _, svc := range services {
		alive := pidAlive(svc.Name)
		listening := false
		if svc.Port > 0 {
			listening = checkPort(svc.Port)
		} else {
			listening = alive
		}
		if listening {
			statuses[svc.Name] = StatusUp
		} else if alive {
			statuses[svc.Name] = StatusStarting
		} else {
			_, err := os.Stat(filepath.Join(pidDir, svc.Name+".pid"))
			if err != nil {
				statuses[svc.Name] = StatusSkipped
			} else {
				statuses[svc.Name] = StatusDown
			}
		}
	}
}

func colorizeLogLine(line string) string {
	lower := strings.ToLower(line)
	switch {
	case strings.Contains(lower, "error") || strings.Contains(lower, "exception") || strings.Contains(lower, "fatal"):
		return fmt.Sprintf("[#f7768e]%s[-]", tview.Escape(line))
	case strings.Contains(lower, "warn"):
		return fmt.Sprintf("[#e0af68]%s[-]", tview.Escape(line))
	case strings.Contains(lower, "started") || strings.Contains(lower, "listening") || strings.Contains(lower, "ready") || strings.Contains(lower, "compiled"):
		return fmt.Sprintf("[#9ece6a]%s[-]", tview.Escape(line))
	case strings.Contains(lower, "200") || strings.Contains(lower, "processing"):
		return fmt.Sprintf("[#7dcfff]%s[-]", tview.Escape(line))
	default:
		return tview.Escape(line)
	}
}

func svcDir(svcIdx int) string {
	if svcIdx < 0 || svcIdx >= len(services) {
		return root
	}
	return filepath.Join(root, services[svcIdx].Dir)
}

// ── Run command ─────────────────────────────────────────────────────────────

func runCmd(app *tview.Application, logView *tview.TextView, svcIdx int, cmdStr string) {
	svc := services[svcIdx]
	dir := svcDir(svcIdx)

	app.QueueUpdateDraw(func() {
		fmt.Fprintf(logView, "\n[#bb9af7::b]▶ Running in %s/:[-::-] [#7dcfff]%s[-]\n", svc.Dir, tview.Escape(cmdStr))
		logView.ScrollToEnd()
	})

	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		app.QueueUpdateDraw(func() {
			fmt.Fprintf(logView, "[#f7768e]Error creating pipe: %s[-]\n", tview.Escape(err.Error()))
		})
		return
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		app.QueueUpdateDraw(func() {
			fmt.Fprintf(logView, "[#f7768e]Error starting command: %s[-]\n", tview.Escape(err.Error()))
		})
		return
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		app.QueueUpdateDraw(func() {
			fmt.Fprintln(logView, colorizeLogLine(line))
			logView.ScrollToEnd()
		})
	}

	err = cmd.Wait()
	app.QueueUpdateDraw(func() {
		if err != nil {
			fmt.Fprintf(logView, "[#f7768e]▶ Command exited: %s[-]\n", tview.Escape(err.Error()))
		} else {
			fmt.Fprintln(logView, "[#9ece6a]▶ Command completed successfully[-]")
		}
		logView.ScrollToEnd()
	})
}

// activeScrollView returns the scrollable text view for the current tab.
func activeScrollView(tab int, logViews []*tview.TextView, errorsView *tview.TextView) *tview.TextView {
	if tab == errorsTabIndex() {
		return errorsView
	}
	if tab > 0 && tab <= len(services) {
		return logViews[tab-1]
	}
	return nil // All tab has no single scroll view
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	exe, _ := os.Executable()
	root = filepath.Dir(exe)
	if filepath.Base(root) == "dev-dashboard" || strings.Contains(root, "go-build") {
		root, _ = filepath.Abs(filepath.Join(root, ".."))
	}
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if _, err := os.Stat(filepath.Join(root, "backend")); err != nil {
		root = filepath.Dir(root)
	}

	logDir = filepath.Join(root, "tmp", "logs")
	pidDir = filepath.Join(root, "tmp", "pids")

	for _, svc := range services {
		statuses[svc.Name] = StatusStarting
		filePos[svc.Name] = 0
	}

	app := tview.NewApplication()

	// ── Header ──────────────────────────────────────────────────────────
	header := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	header.SetBackgroundColor(colorSurface)

	updateHeader := func() {
		header.Clear()
		fmt.Fprintf(header, " [#7dcfff::b]◆ BERTO[-::-] [#565f89]dev dashboard[-]    [#565f89]⏱ %s[-]", elapsed())
	}
	updateHeader()

	// ── Status Bar (tabs) ───────────────────────────────────────────────
	statusBar := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	statusBar.SetBackgroundColor(colorBg)

	updateStatusBar := func() {
		statusBar.Clear()
		mu.Lock()
		defer mu.Unlock()

		parts := []string{"  "}

		// "All" tab first
		if activeTab == allTabIndex {
			parts = append(parts, "[#1a1b26:#7dcfff::b] ◆ All [-:-::-]  ")
		} else {
			parts = append(parts, "[#565f89]  All[-]  ")
		}

		// Service tabs
		for i, svc := range services {
			st := statuses[svc.Name]
			stColor := fmt.Sprintf("#%06x", st.Color().Hex())
			svcColor := fmt.Sprintf("#%06x", svc.Color.Hex())
			tabIdx := svcTabIndex(i)

			portStr := ""
			if svc.Port > 0 {
				portStr = fmt.Sprintf(":%d", svc.Port)
			}

			if activeTab == tabIdx {
				// Selected: inverted colors, bold
				parts = append(parts, fmt.Sprintf(
					"[#1a1b26:%s::b] %s [-:-::-] [%s]%s %s[-] [#565f89]%s[-]  ",
					svcColor, svc.Label, stColor, st.Icon(), st, portStr,
				))
			} else {
				// Unselected: dim
				parts = append(parts, fmt.Sprintf(
					"[%s]%s[-] [#565f89]%s[-] [%s]%s[-] [#565f89]%s[-]  ",
					svcColor, svc.Label, svc.Tech, stColor, st.Icon(), portStr,
				))
			}
		}
		// Errors tab
		if activeTab == errorsTabIndex() {
			parts = append(parts, "[#1a1b26:#f7768e::b] ⚠ Errors [-:-::-]  ")
		} else {
			parts = append(parts, "[#f7768e]⚠ Errors[-]  ")
		}

		fmt.Fprint(statusBar, strings.Join(parts, ""))
	}
	updateStatusBar()

	// ── Log views ───────────────────────────────────────────────────────
	logViews := make([]*tview.TextView, len(services))
	for i, svc := range services {
		tv := tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetChangedFunc(func() { app.Draw() })
		tv.SetBackgroundColor(colorBg)
		tv.SetBorder(true).
			SetBorderColor(svc.Color).
			SetTitle(fmt.Sprintf(" %s ", svc.Label)).
			SetTitleColor(svc.Color).
			SetBorderPadding(0, 0, 1, 1)
		logViews[i] = tv
	}

	// ── Errors view ────────────────────────────────────────────────────
	errorsView := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetChangedFunc(func() { app.Draw() })
	errorsView.SetBackgroundColor(colorBg)
	errorsView.SetBorder(true).
		SetBorderColor(colorRed).
		SetTitle(" Errors & Warnings ").
		SetTitleColor(colorRed).
		SetBorderPadding(0, 0, 1, 1)

	isErrorLine := func(line string) bool {
		lower := strings.ToLower(line)
		return strings.Contains(lower, "error") ||
			strings.Contains(lower, "exception") ||
			strings.Contains(lower, "fatal") ||
			strings.Contains(lower, "warn") ||
			strings.Contains(lower, "fail") ||
			strings.Contains(lower, "refused") ||
			strings.Contains(lower, "timeout") ||
			strings.Contains(lower, "crash")
	}

	// ── Pages for tab switching ─────────────────────────────────────────
	pages := tview.NewPages()

	// Tab 0 = All (2x2 grid)
	allGrid := tview.NewGrid().
		SetRows(0, 0).
		SetColumns(0, 0).
		SetBorders(false)
	allGrid.SetBackgroundColor(colorBg)
	allGrid.AddItem(logViews[0], 0, 0, 1, 1, 0, 0, false)
	allGrid.AddItem(logViews[1], 0, 1, 1, 1, 0, 0, false)
	allGrid.AddItem(logViews[2], 1, 0, 1, 1, 0, 0, false)
	allGrid.AddItem(logViews[3], 1, 1, 1, 1, 0, 0, false)
	pages.AddPage("tab-0", allGrid, true, activeTab == allTabIndex)

	// Tab 1..N = individual services
	for i := range services {
		tabIdx := svcTabIndex(i)
		pages.AddPage(fmt.Sprintf("tab-%d", tabIdx), logViews[i], true, activeTab == tabIdx)
	}

	// Last tab = Errors
	pages.AddPage(fmt.Sprintf("tab-%d", errorsTabIndex()), errorsView, true, activeTab == errorsTabIndex())

	// ── Command input ───────────────────────────────────────────────────
	cmdInput := tview.NewInputField()
	cmdInput.SetBackgroundColor(colorSurface)
	cmdInput.SetFieldBackgroundColor(colorBg)
	cmdInput.SetFieldTextColor(colorText)
	cmdInput.SetLabelColor(colorCyan)
	cmdInput.SetPlaceholder("command to run...")
	cmdInput.SetPlaceholderTextColor(colorDim)

	// ── Footer ──────────────────────────────────────────────────────────
	footer := tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	footer.SetBackgroundColor(colorSurface)

	updateFooter := func() {
		footer.Clear()
		if cmdMode {
			svcIdx := activeTab - 1
			if svcIdx >= 0 && svcIdx < len(services) {
				fmt.Fprintf(footer, " [#bb9af7::b]COMMAND MODE[-::-]  [#565f89]enter[-]=run  [#565f89]esc[-]=cancel  [#565f89]dir:[-] %s", services[svcIdx].Dir)
			}
		} else {
			fmt.Fprint(footer, " [#7dcfff::b]1[-::-]all [#565f89]2[-]back [#565f89]3[-]jobs [#565f89]4[-]front [#565f89]5[-]app [#f7768e]6[-]errors [#565f89]tab[-]cycle [#565f89]↑↓[-]scroll [#e0af68]i[-]iOS [#e0af68]a[-]android [#565f89]:[-][#bb9af7]cmd[-] [#565f89]q[-]quit")
		}
	}
	updateFooter()

	// ── Footer area ─────────────────────────────────────────────────────
	footerArea := tview.NewPages()
	footerArea.AddPage("footer", footer, true, true)
	footerArea.AddPage("cmd", cmdInput, true, false)

	// ── Main layout ─────────────────────────────────────────────────────
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(statusBar, 1, 0, false).
		AddItem(pages, 0, 1, false).
		AddItem(footerArea, 1, 0, false)
	layout.SetBackgroundColor(colorBg)

	// ── Tab switching ───────────────────────────────────────────────────
	switchTab := func(tab int) {
		if tab == activeTab {
			return
		}
		activeTab = tab
		pages.SwitchToPage(fmt.Sprintf("tab-%d", tab))
		updateStatusBar()
		updateFooter()
	}

	// ── Command mode ────────────────────────────────────────────────────
	enterCmdMode := func() {
		if activeTab == allTabIndex || activeTab == errorsTabIndex() {
			switchTab(svcTabIndex(0)) // switch to first service
		}
		svcIdx := activeTab - 1
		cmdMode = true
		cmdInput.SetLabel(fmt.Sprintf(" [%s]%s[-] ▶ ", fmt.Sprintf("#%06x", services[svcIdx].Color.Hex()), services[svcIdx].Label))
		cmdInput.SetText("")
		updateFooter()
		footerArea.SwitchToPage("cmd")
		app.SetFocus(cmdInput)
	}

	exitCmdMode := func() {
		cmdMode = false
		updateFooter()
		footerArea.SwitchToPage("footer")
		app.SetFocus(layout)
	}

	cmdInput.SetDoneFunc(func(key tcell.Key) {
		if key == tcell.KeyEscape {
			exitCmdMode()
			return
		}
		if key == tcell.KeyEnter {
			cmdStr := strings.TrimSpace(cmdInput.GetText())
			if cmdStr == "" {
				exitCmdMode()
				return
			}
			svcIdx := activeTab - 1
			logView := logViews[svcIdx]
			exitCmdMode()
			go runCmd(app, logView, svcIdx, cmdStr)
		}
	})

	// ── Keyboard ────────────────────────────────────────────────────────
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if cmdMode {
			return event
		}

		tc := tabCount()

		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'q', 'Q':
				app.Stop()
				return nil
			case '1':
				switchTab(0) // All
				return nil
			case '2':
				switchTab(1) // Backend
				return nil
			case '3':
				switchTab(2) // Jobs
				return nil
			case '4':
				switchTab(3) // Frontend
				return nil
			case '5':
				switchTab(4) // Mobile App
				return nil
			case '6':
				switchTab(errorsTabIndex()) // Errors
				return nil
			case 'i', 'I':
				// Open app on iOS Simulator
				go func() {
					// Boot simulator if not running
					_ = exec.Command("open", "-a", "Simulator").Run()
					time.Sleep(2 * time.Second)
					// Use Expo CLI to launch on iOS (connects to running dev server)
					cmd := exec.Command("npx", "uri-scheme", "open", "exp://127.0.0.1:8081", "--ios")
					cmd.Dir = filepath.Join(root, "app")
					out, err := cmd.CombinedOutput()
					if err != nil {
						// Fallback: use xcrun to open URL in simulator
						_ = exec.Command("xcrun", "simctl", "openurl", "booted", "exp://127.0.0.1:8081").Run()
					}
					_ = out
				}()
				return nil
			case 'a', 'A':
				// Open app on Android Emulator
				go func() {
					cmd := exec.Command("adb", "shell", "am", "start", "-a", "android.intent.action.VIEW", "-d", "exp://127.0.0.1:8081")
					_ = cmd.Run()
				}()
				return nil
			case ':', ';':
				enterCmdMode()
				return nil
			}
		case tcell.KeyTab:
			switchTab((activeTab + 1) % tc)
			return nil
		case tcell.KeyUp:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				row, col := tv.GetScrollOffset()
				tv.ScrollTo(row-1, col)
			}
			return nil
		case tcell.KeyDown:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				row, col := tv.GetScrollOffset()
				tv.ScrollTo(row+1, col)
			}
			return nil
		case tcell.KeyPgUp:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				row, col := tv.GetScrollOffset()
				tv.ScrollTo(row-20, col)
			}
			return nil
		case tcell.KeyPgDn:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				row, col := tv.GetScrollOffset()
				tv.ScrollTo(row+20, col)
			}
			return nil
		case tcell.KeyEnd:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				tv.ScrollToEnd()
			}
			return nil
		case tcell.KeyHome:
			if tv := activeScrollView(activeTab, logViews, errorsView); tv != nil {
				tv.ScrollToBeginning()
			}
			return nil
		case tcell.KeyCtrlC:
			app.Stop()
			return nil
		}
		return event
	})

	// ── Background: read logs ───────────────────────────────────────────
	go func() {
		for {
			time.Sleep(300 * time.Millisecond)
			for i, svc := range services {
				newText := readNewLines(svc.Name, svc.Log)
				if newText == "" {
					continue
				}
				lines := strings.Split(newText, "\n")
				tv := logViews[i]
				svcLabel := svc.Label
				svcColorHex := fmt.Sprintf("#%06x", svc.Color.Hex())
				app.QueueUpdateDraw(func() {
					for _, line := range lines {
						if line == "" {
							continue
						}
						fmt.Fprintln(tv, colorizeLogLine(line))
						// Also send errors/warnings to the Errors tab
						if isErrorLine(line) {
							fmt.Fprintf(errorsView, "[%s::b]%s[-::-] %s\n", svcColorHex, svcLabel, colorizeLogLine(line))
						}
					}
					tv.ScrollToEnd()
					errorsView.ScrollToEnd()
				})
			}
		}
	}()

	// ── Background: check statuses ──────────────────────────────────────
	go func() {
		for {
			updateStatuses()
			app.QueueUpdateDraw(func() {
				updateHeader()
				updateStatusBar()
			})
			time.Sleep(3 * time.Second)
		}
	}()

	// ── Run ─────────────────────────────────────────────────────────────
	if err := app.SetRoot(layout, true).EnableMouse(false).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

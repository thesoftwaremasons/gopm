// Command gopm is the CLI for the gopm process manager. It sends
// commands to the gopmd daemon over a loopback TCP socket, auto-launching
// the daemon if it is not yet running.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/thesoftwaremasons/gopm/internal/daemon"
	"github.com/thesoftwaremasons/gopm/internal/ipc"
	"github.com/thesoftwaremasons/gopm/internal/web"
	"github.com/thesoftwaremasons/gopm/pkg/config"
)

const usage = `gopm — process manager

Usage:
  gopm [-host addr] <command> [flags]

Commands:
  start <gopm.yaml> [--group <name>]   Load config and start all (or group) apps
  stop <name>                          Stop a process
  restart <name>                       Restart a process
  delete <name>                        Stop and remove a process
  list [--json] [--names]              Show all managed processes
  logs <name|--all> [-n N] [-f]        Tail or follow process logs
       [--since <duration|RFC3339>]    Filter logs by time
       [--grep <pattern>]              Filter logs by substring
  exec <name> -- <cmd> [args...]       Run a command in a process's environment
  validate <gopm.yaml>                 Validate a config file
  web [--port 6120]                    Start web dashboard
  ui                                   Live-refreshing process table (Ctrl+C to quit)
  completion <bash|zsh|powershell>     Print shell completion script
  generate-service [--type systemd|windows]  Generate service installer
  daemon                               Explicitly launch the daemon
  shutdown                             Gracefully stop the daemon

Global flags:
  -host <addr>    Daemon address (default 127.0.0.1:6119)
`

// daemonAddr is the global daemon address, overridden by -host flag.
var daemonAddr = ipc.DefaultAddr

// customHost is true when -host was provided; suppresses ensureDaemonRunning.
var customHost bool

func main() {
	// Parse global -host flag before the subcommand.
	args := os.Args[1:]
	if len(args) >= 2 && args[0] == "-host" {
		daemonAddr = args[1]
		customHost = true
		args = args[2:]
	}

	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := args[0]
	rest := args[1:]

	switch cmd {
	case "start":
		cmdStart(rest)
	case "stop":
		cmdStop(rest)
	case "restart":
		cmdRestart(rest)
	case "delete":
		cmdDelete(rest)
	case "list", "ls":
		cmdList(rest)
	case "logs":
		cmdLogs(rest)
	case "exec":
		cmdExec(rest)
	case "validate":
		cmdValidate(rest)
	case "web":
		cmdWeb(rest)
	case "ui":
		cmdUI(rest)
	case "completion":
		cmdCompletion(rest)
	case "generate-service":
		cmdGenerateService(rest)
	case "daemon":
		if !customHost {
			ensureDaemonRunning()
		}
		fmt.Println("gopmd is running on", daemonAddr)
	case "shutdown":
		cmdShutdown()
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", cmd)
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
}

func cmdStart(args []string) {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	group := fs.String("group", "", "only start apps in this group")
	_ = fs.Parse(args)
	rest := fs.Args()

	if len(rest) < 1 {
		die("start: missing path to gopm.yaml")
	}
	cfgPath, err := filepath.Abs(rest[0])
	if err != nil {
		die(err.Error())
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		die(err.Error())
	}

	// Resolve relative paths in each app against the config's directory so
	// the daemon (which has its own cwd) can launch them correctly.
	cfgDir := filepath.Dir(cfgPath)
	for i := range cfg.Apps {
		a := &cfg.Apps[i]
		if a.Cwd != "" && !filepath.IsAbs(a.Cwd) {
			a.Cwd = filepath.Join(cfgDir, a.Cwd)
		}
		if a.LogFile != "" && !filepath.IsAbs(a.LogFile) {
			a.LogFile = filepath.Join(cfgDir, a.LogFile)
		}
		if a.EnvFile != "" && !filepath.IsAbs(a.EnvFile) {
			a.EnvFile = filepath.Join(cfgDir, a.EnvFile)
		}
	}

	// Feature 8/20: filter by group.
	if *group != "" {
		filtered := cfg.Apps[:0]
		for _, app := range cfg.Apps {
			for _, g := range app.Groups {
				if g == *group {
					filtered = append(filtered, app)
					break
				}
			}
		}
		cfg.Apps = filtered
		if len(cfg.Apps) == 0 {
			die(fmt.Sprintf("start: no apps found in group %q", *group))
		}
	}

	if !customHost {
		ensureDaemonRunning()
	}
	// Use a generous timeout: the daemon queues async restarts but still
	// processes each new app entry before responding, so allow up to 60s.
	resp, err := ipc.NewClientWithTimeout(daemonAddr, 60*time.Second).Send(&ipc.Request{
		Command: ipc.CmdStart,
		Apps:    cfg.Apps,
		Group:   *group,
	})
	checkResp(resp, err)
	fmt.Println(resp.Message)
	cmdList(nil)
}

func cmdStop(args []string) {
	if len(args) < 1 {
		die("stop: missing process name")
	}
	// Use extended timeout since stop waits up to 10s for process termination
	resp, err := ipc.NewClientWithTimeout(daemonAddr, 15*time.Second).Send(&ipc.Request{Command: ipc.CmdStop, AppName: args[0]})
	checkResp(resp, err)
	fmt.Println(resp.Message)
}

func cmdRestart(args []string) {
	if len(args) < 1 {
		die("restart: missing process name")
	}
	// Use extended timeout for stop + start sequence
	resp, err := ipc.NewClientWithTimeout(daemonAddr, 30*time.Second).Send(&ipc.Request{Command: ipc.CmdRestart, AppName: args[0]})
	checkResp(resp, err)
	fmt.Println(resp.Message)
}

func cmdDelete(args []string) {
	if len(args) < 1 {
		die("delete: missing process name")
	}
	resp, err := client().Send(&ipc.Request{Command: ipc.CmdDelete, AppName: args[0]})
	checkResp(resp, err)
	fmt.Println(resp.Message)
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "print JSON")
	namesOnly := fs.Bool("names", false, "print names only, one per line (for shell completion)")
	_ = fs.Parse(args)

	if err := ipc.Ping(daemonAddr); err != nil {
		fmt.Println("daemon not running")
		return
	}
	resp, err := client().Send(&ipc.Request{Command: ipc.CmdList})
	checkResp(resp, err)
	if *namesOnly {
		for _, p := range resp.Processes {
			fmt.Println(p.Name)
		}
		return
	}
	if *jsonOut {
		printJSON(resp.Processes)
		return
	}
	renderList(resp.Processes)
}

func cmdLogs(args []string) {
	name, lines, jsonOut, follow, all, since, grep := parseLogsArgs(args)

	// Feature 4: --all without -f
	if all && !follow {
		// Use extended timeout for reading/filtering logs from all processes
		resp, err := ipc.NewClientWithTimeout(daemonAddr, 30*time.Second).Send(&ipc.Request{
			Command: ipc.CmdLogsAll,
			Lines:   lines,
		})
		checkResp(resp, err)
		for _, line := range resp.Logs {
			fmt.Println(line)
		}
		return
	}

	// Feature 3: follow mode (streaming)
	if follow {
		req := &ipc.Request{
			Command: ipc.CmdLogStream,
			Follow:  true,
			All:     all,
		}
		if !all {
			if name == "" {
				die("logs: missing process name")
			}
			req.AppName = name
		}
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = client().Stream(req, func(ev ipc.LogEvent) bool {
				if all {
					fmt.Printf("[%s] %s\n", ev.Name, ev.Line)
				} else {
					fmt.Println(ev.Line)
				}
				return true
			})
		}()
		select {
		case <-sigCh:
		case <-done:
		}
		return
	}

	if name == "" {
		die("logs: missing process name")
	}
	// Use extended timeout for log filtering operations
	resp, err := ipc.NewClientWithTimeout(daemonAddr, 30*time.Second).Send(&ipc.Request{
		Command: ipc.CmdLogs,
		AppName: name,
		Lines:   lines,
		Since:   since,
		Grep:    grep,
	})
	checkResp(resp, err)
	if jsonOut {
		printJSON(struct {
			Name string   `json:"name"`
			Logs []string `json:"logs"`
		}{
			Name: name,
			Logs: resp.Logs,
		})
		return
	}
	for _, line := range resp.Logs {
		fmt.Println(line)
	}
}

func parseLogsArgs(args []string) (name string, lines int, jsonOut bool, follow bool, all bool, since string, grep string) {
	lines = 50
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			jsonOut = true
		case arg == "-f" || arg == "--follow":
			follow = true
		case arg == "--all" || arg == "-a":
			all = true
		case arg == "-n" || arg == "--lines":
			if i+1 >= len(args) {
				die("logs: " + arg + " requires a value")
			}
			i++
			lines = parsePositiveInt("logs: "+arg, args[i])
		case strings.HasPrefix(arg, "-n="):
			lines = parsePositiveInt("logs: -n", strings.TrimPrefix(arg, "-n="))
		case strings.HasPrefix(arg, "--lines="):
			lines = parsePositiveInt("logs: --lines", strings.TrimPrefix(arg, "--lines="))
		case arg == "--since":
			if i+1 >= len(args) {
				die("logs: --since requires a value")
			}
			i++
			since = args[i]
		case strings.HasPrefix(arg, "--since="):
			since = strings.TrimPrefix(arg, "--since=")
		case arg == "--grep":
			if i+1 >= len(args) {
				die("logs: --grep requires a value")
			}
			i++
			grep = args[i]
		case strings.HasPrefix(arg, "--grep="):
			grep = strings.TrimPrefix(arg, "--grep=")
		case strings.HasPrefix(arg, "-"):
			die("logs: unknown flag " + arg)
		case name == "":
			name = arg
		default:
			die("logs: unexpected argument " + arg)
		}
	}
	return name, lines, jsonOut, follow, all, since, grep
}

// cmdExec runs a command in the context of a named process (exec streaming).
// Usage: gopm exec <name> -- <cmd> [args...]
func cmdExec(args []string) {
	if len(args) < 1 {
		die("exec: missing process name")
	}
	name := args[0]
	rest := args[1:]
	// Strip optional "--" separator.
	if len(rest) > 0 && rest[0] == "--" {
		rest = rest[1:]
	}
	if len(rest) == 0 {
		die("exec: missing command")
	}

	req := &ipc.Request{
		Command:  ipc.CmdExec,
		AppName:  name,
		ExecArgs: rest,
	}
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = client().Stream(req, func(ev ipc.LogEvent) bool {
			fmt.Println(ev.Line)
			return true
		})
	}()
	select {
	case <-sigCh:
	case <-done:
	}
}

func cmdValidate(args []string) {
	if len(args) < 1 {
		die("validate: missing path to gopm.yaml")
	}
	cfgPath, err := filepath.Abs(args[0])
	if err != nil {
		die(err.Error())
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		die(err.Error())
	}
	issues := validateConfigPaths(cfgPath, cfg)
	if len(issues) == 0 {
		fmt.Printf("%s is valid (%d app(s))\n", cfgPath, len(cfg.Apps))
		return
	}
	fmt.Printf("%s parsed, but has %d issue(s):\n", cfgPath, len(issues))
	for _, issue := range issues {
		fmt.Println("  - " + issue)
	}
	os.Exit(1)
}

func cmdShutdown() {
	if err := ipc.Ping(daemonAddr); err != nil {
		fmt.Println("daemon not running")
		return
	}
	resp, err := client().Send(&ipc.Request{Command: ipc.CmdShutdown})
	checkResp(resp, err)
	fmt.Println(resp.Message)
}

// Feature 14: web dashboard.
func cmdWeb(args []string) {
	fs := flag.NewFlagSet("web", flag.ExitOnError)
	port := fs.Int("port", 6120, "HTTP port for the web dashboard")
	_ = fs.Parse(args)

	if err := ipc.Ping(daemonAddr); err != nil {
		die("daemon not running")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := web.New(addr, daemonAddr)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Serve(ctx); err != nil {
		die(err.Error())
	}
}

// Feature 16: live-refresh TUI.
func cmdUI(args []string) {
	_ = args
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case <-ticker.C:
			clearScreen()
			if err := ipc.Ping(daemonAddr); err != nil {
				fmt.Println("daemon not running")
				fmt.Println("\nPress Ctrl+C to exit")
				continue
			}
			resp, err := client().Send(&ipc.Request{Command: ipc.CmdList})
			if err != nil {
				fmt.Println("error:", err)
			} else {
				renderList(resp.Processes)
			}
			fmt.Println("\nPress Ctrl+C to exit  |  gopm restart <name>  |  gopm stop <name>")
		case <-sigCh:
			clearScreen()
			return
		}
	}
}

func clearScreen() {
	fmt.Print("\033[2J\033[H")
}

// Feature 17: shell completion.
func cmdCompletion(args []string) {
	if len(args) < 1 {
		die("completion: specify bash, zsh, or powershell")
	}
	switch strings.ToLower(args[0]) {
	case "bash":
		fmt.Print(`# gopm bash completion
# Add to ~/.bashrc:  source <(gopm completion bash)
_gopm_completion() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local prev="${COMP_WORDS[COMP_CWORD-1]}"
  local cmds="start stop restart delete list ls logs exec validate web ui completion generate-service daemon shutdown"
  case "$prev" in
    stop|restart|delete|logs|exec)
      COMPREPLY=($(compgen -W "$(gopm list --names 2>/dev/null)" -- "$cur"))
      return ;;
    gopm)
      COMPREPLY=($(compgen -W "$cmds" -- "$cur"))
      return ;;
  esac
  COMPREPLY=($(compgen -W "$cmds" -- "$cur"))
}
complete -F _gopm_completion gopm
`)
	case "zsh":
		fmt.Print(`# gopm zsh completion
# Add to ~/.zshrc:  source <(gopm completion zsh)
_gopm() {
  local -a cmds
  cmds=(
    'start:Load config and start apps'
    'stop:Stop a process'
    'restart:Restart a process'
    'delete:Stop and remove a process'
    'list:Show all managed processes'
    'logs:Tail process logs'
    'exec:Run a command in a process env'
    'validate:Validate config file'
    'web:Start web dashboard'
    'ui:Live process table'
    'completion:Shell completion'
    'generate-service:Generate service installer'
    'daemon:Launch daemon'
    'shutdown:Stop daemon'
  )
  if (( CURRENT == 2 )); then
    _describe 'command' cmds
  elif (( CURRENT > 2 )); then
    case "${words[2]}" in
      stop|restart|delete|logs|exec)
        local -a names
        names=(${(f)"$(gopm list --names 2>/dev/null)"})
        _describe 'process' names ;;
    esac
  fi
}
compdef _gopm gopm
`)
	case "powershell":
		fmt.Print(`# gopm PowerShell completion
# Add to $PROFILE:  . <(gopm completion powershell)
Register-ArgumentCompleter -Native -CommandName gopm -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $cmds = @('start','stop','restart','delete','list','ls','logs','exec','validate','web','ui','completion','generate-service','daemon','shutdown')
  $words = $commandAst.CommandElements
  if ($words.Count -ge 3) {
    switch ($words[1]) {
      { $_ -in 'stop','restart','delete','logs','exec' } {
        gopm list --names 2>$null | Where-Object { $_ -like "$wordToComplete*" } |
          ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
        return
      }
    }
  }
  $cmds | Where-Object { $_ -like "$wordToComplete*" } |
    ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}
`)
	default:
		die("completion: unknown shell " + args[0] + " (use bash, zsh, or powershell)")
	}
}

// Feature 18: generate service files.
func cmdGenerateService(args []string) {
	fs := flag.NewFlagSet("generate-service", flag.ExitOnError)
	serviceType := fs.String("type", "", "service type: systemd or windows")
	_ = fs.Parse(args)

	if *serviceType == "" {
		if runtime.GOOS == "windows" {
			*serviceType = "windows"
		} else {
			*serviceType = "systemd"
		}
	}

	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	daemonBin := filepath.Join(dir, "gopmd")
	if runtime.GOOS == "windows" {
		daemonBin += ".exe"
	}

	switch *serviceType {
	case "systemd":
		fmt.Printf(`[Unit]
Description=gopm daemon
After=network.target

[Service]
ExecStart=%s
Restart=always
RestartSec=5
User=%s
WorkingDirectory=%s

[Install]
WantedBy=multi-user.target
`, daemonBin, currentUser(), dir)
		fmt.Fprintf(os.Stderr, "\n# To install:\n#   sudo cp gopmd /usr/local/bin/\n#   sudo cp gopm.service /etc/systemd/system/\n#   sudo systemctl daemon-reload\n#   sudo systemctl enable --now gopm\n")
	case "windows":
		fmt.Printf(`# Install gopmd as a Windows Service using sc.exe:
# (Run these commands as Administrator)

sc create gopmd binPath= "%s" start= auto
sc description gopmd "gopm process manager daemon"
sc start gopmd

# To stop and remove:
#   sc stop gopmd
#   sc delete gopmd
`, daemonBin)
	default:
		die("generate-service: unknown type " + *serviceType + " (use systemd or windows)")
	}
}

func currentUser() string {
	if u := os.Getenv("USER"); u != "" {
		return u
	}
	if u := os.Getenv("USERNAME"); u != "" {
		return u
	}
	return "nobody"
}

func printJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		die("encode json: " + err.Error())
	}
}

func parsePositiveInt(label, value string) int {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		die(label + " must be a positive integer")
	}
	return n
}

func validateConfigPaths(configPath string, cfg *config.GopmConfig) []string {
	var issues []string
	for _, app := range cfg.Apps {
		if app.Cwd != "" {
			cwd := config.ResolvePath(configPath, app.Cwd)
			if info, err := os.Stat(cwd); err != nil {
				issues = append(issues, fmt.Sprintf("%s: cwd %q is not accessible: %v", app.Name, cwd, err))
			} else if !info.IsDir() {
				issues = append(issues, fmt.Sprintf("%s: cwd %q is not a directory", app.Name, cwd))
			}
		}
		if app.LogFile != "" {
			logPath := config.ResolvePath(configPath, app.LogFile)
			logDir := filepath.Dir(logPath)
			if info, err := os.Stat(logDir); err == nil && !info.IsDir() {
				issues = append(issues, fmt.Sprintf("%s: log directory %q is not a directory", app.Name, logDir))
			}
		}
		if app.Watch.Enabled {
			root := config.ResolvePath(configPath, app.Cwd)
			if root == "" {
				root = filepath.Dir(configPath)
			}
			if len(app.Watch.Dirs) == 0 {
				issues = append(issues, fmt.Sprintf("%s: watch.enabled is true but watch.dirs is empty", app.Name))
			}
			for _, dir := range app.Watch.Dirs {
				watchDir := dir
				if !filepath.IsAbs(watchDir) {
					watchDir = filepath.Join(root, watchDir)
				}
				if info, err := os.Stat(watchDir); err != nil {
					issues = append(issues, fmt.Sprintf("%s: watch dir %q is not accessible: %v", app.Name, watchDir, err))
				} else if !info.IsDir() {
					issues = append(issues, fmt.Sprintf("%s: watch dir %q is not a directory", app.Name, watchDir))
				}
			}
		}
	}
	return issues
}

// renderList prints a unicode box-drawn table of processes, with the
// status column color-coded when stdout is a TTY.
func renderList(infos []ipc.ProcessInfo) {
	if len(infos) == 0 {
		fmt.Println("no processes")
		return
	}

	headers := []string{"id", "name", "status", "pid", "restarts", "uptime", "memory", "cpu%", "watching"}
	const statusCol = 2

	rows := make([][]string, len(infos))
	for i, p := range infos {
		pid := "-"
		if p.PID > 0 {
			pid = strconv.Itoa(p.PID)
		}
		watch := "✗"
		if p.Watching {
			watch = "✓"
		}
		cpuStr := "-"
		if p.CPU > 0 {
			cpuStr = fmt.Sprintf("%.1f%%", p.CPU)
		}
		rows[i] = []string{
			strconv.Itoa(p.ID),
			p.Name,
			p.Status,
			pid,
			strconv.Itoa(p.Restarts),
			p.Uptime,
			formatMemory(p.Memory),
			cpuStr,
			watch,
		}
	}

	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if w := utf8.RuneCountInString(cell); w > widths[i] {
				widths[i] = w
			}
		}
	}

	color := isTTY(os.Stdout)
	border := func(left, mid, right string) {
		var b strings.Builder
		b.WriteString(left)
		for i, w := range widths {
			b.WriteString(strings.Repeat("─", w+2))
			if i < len(widths)-1 {
				b.WriteString(mid)
			}
		}
		b.WriteString(right)
		fmt.Println(b.String())
	}

	row := func(cells []string, isHeader bool) {
		var b strings.Builder
		b.WriteString("│")
		for i, cell := range cells {
			padded := padRight(cell, widths[i])
			if !isHeader && i == statusCol && color {
				padded = colorize(cell, widths[i])
			}
			b.WriteString(" ")
			b.WriteString(padded)
			b.WriteString(" │")
		}
		fmt.Println(b.String())
	}

	border("┌", "┬", "┐")
	row(headers, true)
	border("├", "┼", "┤")
	for _, r := range rows {
		row(r, false)
	}
	border("└", "┴", "┘")
}

// padRight returns s padded to width runes (counted, not bytes).
func padRight(s string, width int) string {
	pad := width - utf8.RuneCountInString(s)
	if pad <= 0 {
		return s
	}
	return s + strings.Repeat(" ", pad)
}

// colorize wraps s in an ANSI color code chosen by status, then pads to
// width visible runes (the escape codes don't take visible space).
func colorize(s string, width int) string {
	var code string
	switch s {
	case "online", "ready":
		code = "32" // green
	case "starting", "building":
		code = "33" // yellow
	case "errored", "build_failed", "unhealthy":
		code = "31" // red
	case "stopped":
		code = "90" // grey/dim
	default:
		code = "0"
	}
	pad := width - utf8.RuneCountInString(s)
	if pad < 0 {
		pad = 0
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m" + strings.Repeat(" ", pad)
}

// formatMemory turns a byte count into a short human string. Returns "-"
// for zero (which on non-Linux is the universal answer).
func formatMemory(bytes uint64) string {
	if bytes == 0 {
		return "-"
	}
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
	)
	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/float64(GB))
	case bytes >= MB:
		return fmt.Sprintf("%.1fM", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1fK", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%dB", bytes)
}

// isTTY reports whether f is connected to a terminal. Used to gate ANSI
// color output so redirected stdout stays clean.
func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func client() *ipc.Client {
	return ipc.NewClient(daemonAddr)
}

// ensureDaemonRunning pings the daemon; if unreachable it forks a fresh
// gopmd and waits up to ~2s for it to come up.
func ensureDaemonRunning() {
	if ipc.Ping(daemonAddr) == nil {
		return
	}
	bin, err := findDaemonBinary()
	if err != nil {
		die("could not locate gopmd binary: " + err.Error())
	}
	if err := daemon.Spawn(bin, nil); err != nil {
		die("failed to launch gopmd: " + err.Error())
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ipc.Ping(daemonAddr) == nil {
			return
		}
		time.Sleep(75 * time.Millisecond)
	}
	die("daemon did not become reachable on " + daemonAddr)
}

// findDaemonBinary locates gopmd. Search order:
//  1. $GOPMD env override
//  2. gopmd next to the running gopm binary
//  3. gopmd on PATH
func findDaemonBinary() (string, error) {
	if p := os.Getenv("GOPMD"); p != "" {
		return p, nil
	}
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		candidate := filepath.Join(dir, daemonExeName())
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	if p, err := exec.LookPath(daemonExeName()); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("gopmd not found (set $GOPMD, place it next to gopm, or add to PATH)")
}

func daemonExeName() string {
	if runtime.GOOS == "windows" {
		return "gopmd.exe"
	}
	return "gopmd"
}

func checkResp(resp *ipc.Response, err error) {
	if err != nil {
		die(err.Error())
	}
	if !resp.OK {
		die(resp.Error)
	}
}

func die(msg string) {
	fmt.Fprintln(os.Stderr, "gopm:", msg)
	os.Exit(1)
}

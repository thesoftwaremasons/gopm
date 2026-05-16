package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/thesoftwaremasons/gopm/internal/ipc"
	"github.com/thesoftwaremasons/gopm/internal/logger"
	"github.com/thesoftwaremasons/gopm/internal/stats"
)

// Handler returns an ipc.Handler closure that dispatches incoming requests
// to this supervisor.
func (s *Supervisor) Handler() ipc.Handler {
	return func(ctx context.Context, req *ipc.Request) *ipc.Response {
		switch req.Command {
		case ipc.CmdPing:
			return &ipc.Response{OK: true, Message: "pong"}

		case ipc.CmdStart:
			if len(req.Apps) == 0 && req.Config == nil {
				return errResp("start: no app config provided")
			}
			started := 0
			if req.Config != nil {
				if err := s.StartApp(*req.Config); err != nil {
					return errResp(err.Error())
				}
				started++
			}
			for _, app := range req.Apps {
				if err := s.StartApp(app); err != nil {
					return errResp(err.Error())
				}
				started++
			}
			return &ipc.Response{OK: true, Message: fmt.Sprintf("started %d app(s)", started)}

		case ipc.CmdStop:
			if req.AppName == "" {
				return errResp("stop: app name required")
			}
			if err := s.Stop(req.AppName); err != nil {
				return errResp(err.Error())
			}
			return &ipc.Response{OK: true, Message: "stopped " + req.AppName}

		case ipc.CmdRestart:
			if req.AppName == "" {
				return errResp("restart: app name required")
			}
			if err := s.Restart(req.AppName); err != nil {
				return errResp(err.Error())
			}
			return &ipc.Response{OK: true, Message: "restarted " + req.AppName}

		case ipc.CmdDelete:
			if req.AppName == "" {
				return errResp("delete: app name required")
			}
			if err := s.Delete(req.AppName); err != nil {
				return errResp(err.Error())
			}
			return &ipc.Response{OK: true, Message: "deleted " + req.AppName}

		case ipc.CmdList:
			return &ipc.Response{OK: true, Processes: s.listInfos()}

		case ipc.CmdLogs:
			if req.AppName == "" {
				return errResp("logs: app name required")
			}
			if _, ok := s.lookup(req.AppName); !ok {
				return errResp("no such process: " + req.AppName)
			}
			n := req.Lines
			if n <= 0 {
				n = 50
			}
			// Feature 13: since/grep filtering.
			if req.Since != "" || req.Grep != "" {
				var sinceTime time.Time
				if req.Since != "" {
					if d, err := time.ParseDuration(req.Since); err == nil {
						sinceTime = time.Now().Add(-d)
					} else if t, err := time.Parse(time.RFC3339, req.Since); err == nil {
						sinceTime = t
					}
				}
				lines := s.logger.TailFiltered(req.AppName, n, sinceTime, req.Grep)
				return &ipc.Response{OK: true, Logs: lines}
			}
			return &ipc.Response{OK: true, Logs: s.logger.Tail(req.AppName, n)}

		case ipc.CmdLogsAll:
			// Feature 4: snapshot logs from all processes with [name] prefix.
			n := req.Lines
			if n <= 0 {
				n = 20
			}
			procs := s.List()
			var allLogs []string
			for _, mp := range procs {
				lines := s.logger.Tail(mp.Config.Name, n)
				for _, line := range lines {
					allLogs = append(allLogs, "["+mp.Config.Name+"] "+line)
				}
			}
			return &ipc.Response{OK: true, Logs: allLogs}

		default:
			return errResp("unknown command: " + string(req.Command))
		}
	}
}

// StreamHandler returns an ipc.StreamHandler for CmdLogStream and CmdExec.
func (s *Supervisor) StreamHandler() ipc.StreamHandler {
	return func(ctx context.Context, req *ipc.Request, conn net.Conn) {
		switch req.Command {
		case ipc.CmdLogStream:
			s.handleLogStream(ctx, req, conn)
		case ipc.CmdExec:
			s.handleExec(ctx, req, conn)
		}
	}
}

// handleLogStream subscribes to process logs and writes LogEvent JSON lines
// to conn until ctx is cancelled or a write fails.
func (s *Supervisor) handleLogStream(ctx context.Context, req *ipc.Request, conn net.Conn) {
	enc := json.NewEncoder(conn)

	var ch chan logger.LogEvent
	if req.All {
		raw := s.logger.SubscribeAll()
		defer s.logger.UnsubscribeAll(raw)
		ch = raw
	} else {
		if req.AppName == "" {
			return
		}
		raw := s.logger.Subscribe(req.AppName)
		defer s.logger.Unsubscribe(req.AppName, raw)
		ch = raw
	}

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			if err := enc.Encode(ipc.LogEvent{Name: ev.Name, Line: ev.Line}); err != nil {
				return
			}
		}
	}
}

// handleExec runs a command in the context of the named process and streams
// its output as LogEvent JSON lines.
func (s *Supervisor) handleExec(ctx context.Context, req *ipc.Request, conn net.Conn) {
	if req.AppName == "" || len(req.ExecArgs) == 0 {
		enc := json.NewEncoder(conn)
		_ = enc.Encode(ipc.LogEvent{Name: "gopm", Line: "[error] exec: app name and args required"})
		return
	}
	mp, ok := s.lookup(req.AppName)
	if !ok {
		enc := json.NewEncoder(conn)
		_ = enc.Encode(ipc.LogEvent{Name: "gopm", Line: "[error] no such process: " + req.AppName})
		return
	}

	mp.mu.RLock()
	cwd := mp.Config.Cwd
	cfgEnv := mp.Config.Env
	envFile := mp.Config.EnvFile
	mp.mu.RUnlock()

	// Build the command.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, req.ExecArgs[0], req.ExecArgs[1:]...)
	} else {
		cmd = exec.CommandContext(ctx, req.ExecArgs[0], req.ExecArgs[1:]...)
	}
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Merge env.
	envVars := os.Environ()
	if envFile != "" {
		envFilePath := envFile
		if !isAbsPath(envFilePath) && cwd != "" {
			envFilePath = cwd + string(os.PathSeparator) + envFilePath
		}
		if fe, err := parseEnvFile(envFilePath); err == nil {
			for k, v := range fe {
				if _, ok := cfgEnv[k]; !ok {
					envVars = append(envVars, k+"="+v)
				}
			}
		}
	}
	for k, v := range cfgEnv {
		envVars = append(envVars, k+"="+v)
	}
	cmd.Env = envVars

	enc := json.NewEncoder(conn)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		_ = enc.Encode(ipc.LogEvent{Name: req.AppName, Line: "[error] " + err.Error()})
		return
	}

	// Stream stdout and stderr concurrently.
	done := make(chan struct{}, 2)
	stream := func(r interface{ Read([]byte) (int, error) }, prefix string) {
		buf := make([]byte, 4096)
		var leftover []byte
		for {
			n, err := r.Read(buf)
			if n > 0 {
				data := append(leftover, buf[:n]...)
				leftover = leftover[:0]
				for {
					idx := indexByte(data, '\n')
					if idx < 0 {
						leftover = append([]byte(nil), data...)
						break
					}
					line := strings.TrimRight(string(data[:idx]), "\r")
					if prefix != "" {
						line = prefix + line
					}
					_ = enc.Encode(ipc.LogEvent{Name: req.AppName, Line: line})
					data = data[idx+1:]
				}
			}
			if err != nil {
				if len(leftover) > 0 {
					line := strings.TrimRight(string(leftover), "\r")
					if prefix != "" {
						line = prefix + line
					}
					_ = enc.Encode(ipc.LogEvent{Name: req.AppName, Line: line})
				}
				break
			}
		}
		done <- struct{}{}
	}
	go stream(stdout, "")
	go stream(stderr, "[stderr] ")
	<-done
	<-done
	_ = cmd.Wait()
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// ListInfos is the public version of listInfos for use by the web server.
func (s *Supervisor) ListInfos() []ipc.ProcessInfo {
	return s.listInfos()
}

func (s *Supervisor) listInfos() []ipc.ProcessInfo {
	procs := s.List()
	out := make([]ipc.ProcessInfo, 0, len(procs))
	now := time.Now()
	for _, mp := range procs {
		status, pid, restarts, startedAt := mp.Snapshot()
		uptime := "-"
		if (status == StatusOnline || status == StatusReady || status == StatusUnhealthy) && !startedAt.IsZero() {
			uptime = formatUptime(now.Sub(startedAt))
		}
		pidVal := pid
		if status != StatusOnline && status != StatusReady && status != StatusUnhealthy {
			pidVal = 0
		}
		var mem uint64
		var cpu float64
		if pidVal > 0 {
			mem = stats.MemoryRSS(pidVal)
			cpu = stats.CPUPercent(pidVal)
		}
		out = append(out, ipc.ProcessInfo{
			ID:       mp.ID,
			Name:     mp.Config.Name,
			Status:   string(status),
			PID:      pidVal,
			Restarts: restarts,
			Uptime:   uptime,
			Watching: mp.Config.Watch.Enabled,
			Memory:   mem,
			CPU:      cpu,
		})
	}
	return out
}

func errResp(msg string) *ipc.Response {
	return &ipc.Response{OK: false, Error: msg}
}

func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) - h*60
		return fmt.Sprintf("%dh%dm", h, m)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) - days*24
	return fmt.Sprintf("%dd%dh", days, h)
}

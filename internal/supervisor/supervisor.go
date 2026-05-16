// Package supervisor manages the lifecycle of all gopm-managed processes.
// One Supervisor instance lives for the lifetime of the daemon.
package supervisor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thesoftwaremasons/gopm/internal/healthcheck"
	"github.com/thesoftwaremasons/gopm/internal/logger"
	"github.com/thesoftwaremasons/gopm/internal/process"
	"github.com/thesoftwaremasons/gopm/internal/state"
	"github.com/thesoftwaremasons/gopm/internal/stats"
	"github.com/thesoftwaremasons/gopm/internal/watcher"
	"github.com/thesoftwaremasons/gopm/pkg/config"
)

// Supervisor owns the registry of managed processes and serves as the
// command sink for IPC requests.
type Supervisor struct {
	mu      sync.Mutex
	procs   map[string]*ManagedProcess
	nextID  int
	ctx     context.Context
	cancel  context.CancelFunc
	stopped bool

	logger *logger.Logger
	store  *state.Store
}

// New creates a Supervisor. If store is non-nil, registry changes are
// persisted to it.
func New(store *state.Store) *Supervisor {
	ctx, cancel := context.WithCancel(context.Background())
	return &Supervisor{
		procs:  make(map[string]*ManagedProcess),
		ctx:    ctx,
		cancel: cancel,
		logger: logger.New(0),
		store:  store,
	}
}

// Logger returns the supervisor's underlying log registry. Used by the IPC
// handler to serve `gopm logs`.
func (s *Supervisor) Logger() *logger.Logger { return s.logger }

// persist snapshots the registry and writes it to the store. Errors are
// logged rather than returned so a write failure can never block a
// command.
func (s *Supervisor) persist() {
	if s.store == nil {
		return
	}
	s.mu.Lock()
	apps := make([]state.PersistedApp, 0, len(s.procs))
	for _, mp := range s.procs {
		apps = append(apps, state.PersistedApp{
			Config:   mp.Config,
			Restarts: mp.restartCount(),
		})
	}
	s.mu.Unlock()
	if err := s.store.Save(&state.State{Apps: apps}); err != nil {
		log.Printf("supervisor: persist state: %v", err)
	}
}

// Shutdown asks every managed process to stop and waits for their supervise
// goroutines to exit.
func (s *Supervisor) Shutdown() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	procs := make([]*ManagedProcess, 0, len(s.procs))
	for _, mp := range s.procs {
		procs = append(procs, mp)
	}
	s.mu.Unlock()

	for _, mp := range procs {
		_ = s.stopProcess(mp, true)
	}
	s.cancel()
}

// StartApp registers a new app and starts supervising it. If an app with
// the same name already exists, its config is replaced and it is restarted.
func (s *Supervisor) StartApp(cfg config.AppConfig) error {
	s.mu.Lock()
	if existing, ok := s.procs[cfg.Name]; ok {
		s.mu.Unlock()
		// Replace config and restart asynchronously so the IPC handler can
		// return immediately even when many processes are being restarted.
		existing.mu.Lock()
		existing.Config = cfg
		existing.mu.Unlock()
		go func(name string) {
			if err := s.Restart(name); err != nil {
				log.Printf("supervisor: restart %s: %v", name, err)
			}
		}(cfg.Name)
		return nil
	}

	// Feature 10: port conflict detection — only for new processes.
	// Existing processes are expected to own their port; checking would always
	// produce a false-positive warning.
	if cfg.Port > 0 {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.Port))
		if err != nil {
			s.logger.Write(cfg.Name, fmt.Sprintf("[gopm] WARNING: port %d already in use — process may fail to bind", cfg.Port))
		} else {
			ln.Close()
		}
	}

	mp := newManagedProcess(s.nextID, cfg)
	s.nextID++
	s.procs[cfg.Name] = mp
	s.mu.Unlock()

	if err := s.logger.SetLogFile(cfg.Name, cfg.LogFile, cfg.LogMaxSizeMB, cfg.LogMaxBackups); err != nil {
		log.Printf("supervisor: %s: %v", cfg.Name, err)
	}

	go s.supervise(mp)
	s.persist()
	return nil
}

// Stop signals the named process to stop. It returns an error if the
// process is unknown.
func (s *Supervisor) Stop(name string) error {
	mp, ok := s.lookup(name)
	if !ok {
		return fmt.Errorf("no such process: %s", name)
	}
	return s.stopProcess(mp, false)
}

// Restart stops and re-supervises the named process.
func (s *Supervisor) Restart(name string) error {
	mp, ok := s.lookup(name)
	if !ok {
		return fmt.Errorf("no such process: %s", name)
	}
	// Prevent two concurrent Restart calls from both spawning a new supervise
	// goroutine, which would cause two processes to race for the same port.
	// If a restart is already in-flight we drop this one; the in-flight restart
	// will use the already-updated Config.
	if !mp.restartMu.TryLock() {
		return nil
	}
	defer mp.restartMu.Unlock()

	if err := s.stopProcess(mp, false); err != nil {
		return err
	}
	// Replace stop/done channels and re-supervise with the same config.
	mp.mu.Lock()
	mp.stopCh = make(chan struct{})
	mp.done = make(chan struct{})
	mp.restarts = 0
	mp.crashNotified = false
	mp.mu.Unlock()
	go s.supervise(mp)
	return nil
}

// Delete stops and removes the named process from the registry.
func (s *Supervisor) Delete(name string) error {
	mp, ok := s.lookup(name)
	if !ok {
		return fmt.Errorf("no such process: %s", name)
	}
	if err := s.stopProcess(mp, true); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.procs, name)
	s.mu.Unlock()
	s.logger.Detach(name)
	s.persist()
	return nil
}

// List returns a stable snapshot of every managed process, sorted by ID.
func (s *Supervisor) List() []*ManagedProcess {
	s.mu.Lock()
	out := make([]*ManagedProcess, 0, len(s.procs))
	for _, mp := range s.procs {
		out = append(out, mp)
	}
	s.mu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Supervisor) lookup(name string) (*ManagedProcess, bool) {
	s.mu.Lock()
	mp, ok := s.procs[name]
	s.mu.Unlock()
	return mp, ok
}

func (s *Supervisor) stopProcess(mp *ManagedProcess, force bool) error {
	mp.mu.Lock()
	select {
	case <-mp.stopCh:
		// already stopping
	default:
		close(mp.stopCh)
	}
	cmd := mp.cmd
	mp.mu.Unlock()

	if cmd != nil {
		_ = process.Kill(cmd, force)
	}
	// Wait for supervise loop to acknowledge the stop.
	// Increased timeout to accommodate two-stage kill (graceful + force)
	select {
	case <-mp.done:
	case <-time.After(30 * time.Second):
		return errors.New("timeout waiting for process to stop")
	}
	return nil
}

// waitForDeps blocks until all declared dependencies are online/ready, or until
// the process is asked to stop. Returns true if deps are ready, false if stopped.
func (s *Supervisor) waitForDeps(mp *ManagedProcess) bool {
	if len(mp.Config.DependsOn) == 0 {
		return true
	}
	s.logger.Write(mp.Config.Name, fmt.Sprintf("[gopm] waiting for dependencies: %v", mp.Config.DependsOn))
	for {
		ready := true
		for _, dep := range mp.Config.DependsOn {
			depProc, ok := s.lookup(dep)
			if !ok {
				ready = false
				break
			}
			st := depProc.Status()
			if st != StatusOnline && st != StatusReady {
				ready = false
				break
			}
		}
		if ready {
			return true
		}
		select {
		case <-mp.stopCh:
			mp.setStatus(StatusStopped)
			return false
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// runCommand runs a shell command (like pre_start) and streams output to the
// process log. Returns true on success.
func (s *Supervisor) runCommand(mp *ManagedProcess, shellCmd string) bool {
	s.logger.Write(mp.Config.Name, "[gopm] running: "+shellCmd)
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(s.ctx, "cmd", "/C", shellCmd)
	} else {
		c = exec.CommandContext(s.ctx, "sh", "-c", shellCmd)
	}
	if mp.Config.Cwd != "" {
		c.Dir = mp.Config.Cwd
	}
	stdout, _ := c.StdoutPipe()
	stderr, _ := c.StderrPipe()
	if err := c.Start(); err != nil {
		s.logger.Write(mp.Config.Name, "[gopm] command start failed: "+err.Error())
		return false
	}
	s.logger.Attach(mp.Config.Name, stdout, stderr)
	if err := c.Wait(); err != nil {
		s.logger.Write(mp.Config.Name, "[gopm] command failed: "+err.Error())
		return false
	}
	return true
}

// supervise is the per-process control loop. It mirrors the reference
// implementation in the spec, plus the file-watching / rebuild handling
// from Phase 4, plus all new features.
func (s *Supervisor) supervise(mp *ManagedProcess) {
	defer close(mp.done)

	// Feature 6: depends_on startup ordering.
	if !s.waitForDeps(mp) {
		return
	}

	// Feature 2: pre_start hook — run once before the main loop.
	if mp.Config.PreStart != "" {
		if !s.runCommand(mp, mp.Config.PreStart) {
			s.logger.Write(mp.Config.Name, "[gopm] pre_start failed, continuing anyway")
		}
	}

	// Spin up a file watcher if the app opted in.
	var w *watcher.Watcher
	if mp.Config.Watch.Enabled {
		var err error
		w, err = watcher.New(mp.Config.Watch, mp.Config.Cwd, mp.rebuildCh, func(msg string) {
			s.logger.Write(mp.Config.Name, "[watcher] "+msg)
		})
		if err != nil {
			s.logger.Write(mp.Config.Name, "[watcher] init failed: "+err.Error())
			w = nil
		} else {
			go w.Start()
			defer w.Close()
		}
	}

	for {
		select {
		case <-mp.stopCh:
			mp.setStatus(StatusStopped)
			return
		default:
		}

		cmd := s.buildCmd(mp)
		stdout, errOut := cmd.StdoutPipe()
		if errOut != nil {
			s.logger.Write(mp.Config.Name, "[gopm] stdout pipe error: "+errOut.Error())
		}
		stderr, errErr := cmd.StderrPipe()
		if errErr != nil {
			s.logger.Write(mp.Config.Name, "[gopm] stderr pipe error: "+errErr.Error())
		}

		mp.setStatus(StatusStarting)
		if err := cmd.Start(); err != nil {
			s.logger.Write(mp.Config.Name, "[gopm] start failed: "+err.Error())
			mp.setStatus(StatusErrored)
			if !mp.Config.AutoRestart || s.exhaustedRestarts(mp) {
				if mp.Config.Watch.Enabled {
					if !s.waitRebuildOrStop(mp) {
						return
					}
					if !s.buildCycle(mp) {
						return
					}
					continue
				}
				return
			}
			mp.mu.Lock()
			mp.restarts++
			mp.mu.Unlock()
			if s.sleep(backoff(mp.restartCount()), mp.stopCh) {
				return
			}
			continue
		}

		s.logger.Attach(mp.Config.Name, stdout, stderr)

		// Atomically publish the running cmd and check whether a stop was
		// requested while we were starting.
		mp.mu.Lock()
		mp.cmd = cmd
		mp.pid = cmd.Process.Pid
		mp.startedAt = time.Now()
		stopRequested := false
		select {
		case <-mp.stopCh:
			stopRequested = true
		default:
		}
		mp.mu.Unlock()

		if stopRequested {
			// Two-stage kill for early stop as well
			_ = process.Kill(cmd, false)
			waitCh := make(chan struct{})
			go func() {
				_ = cmd.Wait()
				close(waitCh)
			}()
			select {
			case <-waitCh:
				// Process exited cleanly
			case <-time.After(5 * time.Second):
				// Force kill if graceful stop timed out
				_ = process.Kill(cmd, true)
				<-waitCh
			}
			mp.setStatus(StatusStopped)
			return
		}

		mp.setStatus(StatusOnline)
		currentPID := cmd.Process.Pid
		// Feature 5: track CPU for this PID.
		stats.TrackPID(currentPID)

		// Feature 7: start health check if configured.
		var hcStopFn func()
		var hcStatusCh <-chan healthcheck.Status
		if mp.Config.HealthCheck.URL != "" {
			interval := parseDurationDefault(mp.Config.HealthCheck.Interval, 5*time.Second)
			timeout := parseDurationDefault(mp.Config.HealthCheck.Timeout, 2*time.Second)
			retries := mp.Config.HealthCheck.Retries
			if retries <= 0 {
				retries = 3
			}
			hc := healthcheck.New(mp.Config.HealthCheck.URL, interval, timeout, retries)
			hcStatusCh = hc.Start(s.ctx)
			hcStopFn = hc.Stop
		}

		// Wait on events: child exit, stop request, rebuild, health check.
		waitDone := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(waitDone)
		}()

		event := s.waitEvent(mp, waitDone, hcStatusCh)

		// Stop health checker and CPU tracking.
		if hcStopFn != nil {
			hcStopFn()
		}
		stats.UntrackPID(currentPID)

		switch event {
		case "stop":
			// Two-stage kill: try graceful first, then force after timeout
			_ = process.Kill(cmd, false)
			select {
			case <-waitDone:
				// Process exited cleanly
			case <-time.After(5 * time.Second):
				// Graceful stop timed out, force kill
				s.logger.Write(mp.Config.Name, "[gopm] graceful stop timed out, forcing termination")
				_ = process.Kill(cmd, true)
				<-waitDone
			}
			mp.setStatus(StatusStopped)
			return

		case "rebuild":
			// Two-stage kill for rebuild as well
			_ = process.Kill(cmd, false)
			select {
			case <-waitDone:
				// Process exited cleanly
			case <-time.After(5 * time.Second):
				// Force kill if graceful stop timed out
				_ = process.Kill(cmd, true)
				<-waitDone
			}
			if !s.buildCycle(mp) {
				return
			}
			mp.mu.Lock()
			mp.restarts = 0
			mp.mu.Unlock()
			continue

		case "exit":
			if !mp.Config.AutoRestart {
				mp.setStatus(StatusStopped)
				if mp.Config.Watch.Enabled {
					if !s.waitRebuildOrStop(mp) {
						return
					}
					if !s.buildCycle(mp) {
						return
					}
					continue
				}
				return
			}
			if s.exhaustedRestarts(mp) {
				mp.setStatus(StatusErrored)
				// Feature 12: crash webhook.
				mp.mu.Lock()
				notified := mp.crashNotified
				if !notified {
					mp.crashNotified = true
				}
				mp.mu.Unlock()
				if !notified && mp.Config.OnCrash.Webhook != "" {
					go sendCrashWebhook(mp.Config.Name, mp.Config.OnCrash.Webhook)
				}
				if mp.Config.Watch.Enabled {
					if !s.waitRebuildOrStop(mp) {
						return
					}
					if !s.buildCycle(mp) {
						return
					}
					continue
				}
				return
			}
			mp.mu.Lock()
			mp.restarts++
			mp.crashNotified = false // reset on retry
			mp.mu.Unlock()
			if s.sleep(backoff(mp.restartCount()), mp.stopCh) {
				return
			}
		}
	}
}

// waitEvent blocks until one of: process exits, stop requested, rebuild
// signal, or health status change. Health status changes are applied directly
// via setStatus. Returns "exit", "stop", or "rebuild".
func (s *Supervisor) waitEvent(mp *ManagedProcess, waitDone <-chan struct{}, hcStatusCh <-chan healthcheck.Status) string {
	for {
		if hcStatusCh != nil {
			select {
			case <-waitDone:
				return "exit"
			case <-mp.stopCh:
				return "stop"
			case <-mp.rebuildCh:
				return "rebuild"
			case st, ok := <-hcStatusCh:
				if !ok {
					hcStatusCh = nil
					continue
				}
				switch st {
				case healthcheck.StatusReady:
					mp.setStatus(StatusReady)
				case healthcheck.StatusUnhealthy:
					mp.setStatus(StatusUnhealthy)
				}
			}
		} else {
			select {
			case <-waitDone:
				return "exit"
			case <-mp.stopCh:
				return "stop"
			case <-mp.rebuildCh:
				return "rebuild"
			}
		}
	}
}

// sendCrashWebhook fires a one-shot HTTP POST to the configured webhook URL.
func sendCrashWebhook(name, url string) {
	payload := fmt.Sprintf(`{"event":"crash","app":"%s","time":"%s"}`,
		name, time.Now().Format(time.RFC3339))
	resp, err := http.Post(url, "application/json", strings.NewReader(payload)) //nolint:noctx
	if err != nil {
		log.Printf("supervisor: crash webhook %s: %v", name, err)
		return
	}
	_ = resp.Body.Close()
}

// parseDurationDefault parses s as a duration, returning def on any error.
func parseDurationDefault(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

// buildCycle runs the configured build_cmd, retrying on each new file change
// until it succeeds or a stop is requested. Returns true when the build
// succeeded (caller should proceed to start the binary), false when the
// process was asked to stop.
func (s *Supervisor) buildCycle(mp *ManagedProcess) bool {
	for {
		if s.runBuild(mp) {
			return true
		}
		mp.setStatus(StatusBuildFailed)
		if !s.waitRebuildOrStop(mp) {
			return false
		}
	}
}

// runBuild executes the configured build command, streaming its output
// into the process log. Returns true on a clean exit (or when no build
// command is configured), false otherwise.
func (s *Supervisor) runBuild(mp *ManagedProcess) bool {
	buildCmd := mp.Config.Watch.BuildCmd
	if buildCmd == "" {
		return true
	}
	mp.setStatus(StatusBuilding)
	s.logger.Write(mp.Config.Name, "[gopm] building: "+buildCmd)

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(s.ctx, "cmd", "/C", buildCmd)
	} else {
		c = exec.CommandContext(s.ctx, "sh", "-c", buildCmd)
	}
	if mp.Config.Cwd != "" {
		c.Dir = mp.Config.Cwd
	}
	stdout, _ := c.StdoutPipe()
	stderr, _ := c.StderrPipe()

	if err := c.Start(); err != nil {
		s.logger.Write(mp.Config.Name, "[gopm] build start failed: "+err.Error())
		return false
	}
	s.logger.Attach(mp.Config.Name, stdout, stderr)
	if err := c.Wait(); err != nil {
		s.logger.Write(mp.Config.Name, "[gopm] build failed: "+err.Error())
		return false
	}
	s.logger.Write(mp.Config.Name, "[gopm] build ok")
	return true
}

// waitRebuildOrStop blocks until either a rebuild signal arrives (returns
// true) or a stop is requested (returns false).
func (s *Supervisor) waitRebuildOrStop(mp *ManagedProcess) bool {
	select {
	case <-mp.stopCh:
		mp.setStatus(StatusStopped)
		return false
	case <-mp.rebuildCh:
		return true
	}
}

// buildCmd constructs the exec.Cmd for a managed process, incorporating
// env_file support (feature 1).
func (s *Supervisor) buildCmd(mp *ManagedProcess) *exec.Cmd {
	cfg := mp.Config
	cmd := exec.CommandContext(s.ctx, cfg.Command, cfg.Args...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}

	// Build env: start from OS env, layer env_file, then inline Env (highest precedence).
	baseEnv := os.Environ()
	extraEnv := make([]string, 0)

	// Feature 1: parse env_file.
	if cfg.EnvFile != "" {
		envFilePath := cfg.EnvFile
		if !isAbsPath(envFilePath) && cfg.Cwd != "" {
			envFilePath = cfg.Cwd + string(os.PathSeparator) + envFilePath
		}
		if fileEnv, err := parseEnvFile(envFilePath); err == nil {
			for k, v := range fileEnv {
				// Only add if not overridden by inline Env.
				if _, overridden := cfg.Env[k]; !overridden {
					extraEnv = append(extraEnv, k+"="+v)
				}
			}
		} else {
			log.Printf("supervisor: %s: env_file %s: %v", cfg.Name, cfg.EnvFile, err)
		}
	}

	// Inline Env always wins.
	for k, v := range cfg.Env {
		extraEnv = append(extraEnv, k+"="+v)
	}

	if len(extraEnv) > 0 {
		cmd.Env = append(baseEnv, extraEnv...)
	}

	process.SetSysProcAttr(cmd)
	return cmd
}

// isAbsPath is a cross-platform absolute path check.
func isAbsPath(p string) bool {
	if len(p) == 0 {
		return false
	}
	if p[0] == '/' {
		return true
	}
	// Windows: C:\... or \\...
	if len(p) >= 3 && p[1] == ':' {
		return true
	}
	if len(p) >= 2 && p[0] == '\\' && p[1] == '\\' {
		return true
	}
	return false
}

// parseEnvFile parses a KEY=VALUE env file, skipping blank lines and # comments.
func parseEnvFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	result := make(map[string]string)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		// Strip optional surrounding quotes.
		if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') ||
			(val[0] == '\'' && val[len(val)-1] == '\'')) {
			val = val[1 : len(val)-1]
		}
		result[key] = val
	}
	return result, sc.Err()
}

func (s *Supervisor) exhaustedRestarts(mp *ManagedProcess) bool {
	if mp.Config.MaxRestarts <= 0 {
		return false
	}
	return mp.restartCount() >= mp.Config.MaxRestarts
}

// sleep waits for d, returning true if the process or the daemon was asked
// to stop. stopCh should be the calling process's current stopCh so that a
// Stop/Restart request can interrupt a backoff sleep immediately.
func (s *Supervisor) sleep(d time.Duration, stopCh <-chan struct{}) bool {
	if d <= 0 {
		return false
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return false
	case <-s.ctx.Done():
		return true
	case <-stopCh:
		return true
	}
}

func (m *ManagedProcess) restartCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.restarts
}

func backoff(restarts int) time.Duration {
	d := time.Duration(restarts) * 500 * time.Millisecond
	if d > 15*time.Second {
		return 15 * time.Second
	}
	return d
}

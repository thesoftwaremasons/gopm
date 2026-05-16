package supervisor

import (
	"os/exec"
	"sync"
	"time"

	"github.com/thesoftwaremasons/gopm/pkg/config"
)

// Status is the lifecycle state of a managed process.
type Status string

// Recognised status values. The set matches what the spec documents and
// what `gopm list` renders.
const (
	StatusStarting    Status = "starting"
	StatusOnline      Status = "online"
	StatusStopped     Status = "stopped"
	StatusErrored     Status = "errored"
	StatusBuilding    Status = "building"
	StatusBuildFailed Status = "build_failed"
	StatusReady       Status = "ready"
	StatusUnhealthy   Status = "unhealthy"
)

// ManagedProcess is a single supervised process. The supervise goroutine is
// the only writer of mutable state; readers (e.g. the list RPC) take the
// mutex.
type ManagedProcess struct {
	ID     int
	Config config.AppConfig

	mu        sync.RWMutex
	status    Status
	cmd       *exec.Cmd
	pid       int
	startedAt time.Time
	restarts  int

	// crashNotified tracks whether a crash webhook has already fired for the
	// current errored state so we only fire once per failure.
	crashNotified bool

	// restartMu serialises concurrent Restart calls so two callers cannot
	// both spin up a new supervise goroutine for the same process.
	restartMu sync.Mutex

	// stopCh is closed by Stop to ask the supervise loop to exit cleanly
	// after the current child terminates.
	stopCh chan struct{}
	// rebuildCh receives one signal per debounced file-change burst.
	rebuildCh chan struct{}
	// done is closed once the supervise goroutine returns.
	done chan struct{}
}

// newManagedProcess constructs a ManagedProcess for the given app.
func newManagedProcess(id int, cfg config.AppConfig) *ManagedProcess {
	return &ManagedProcess{
		ID:        id,
		Config:    cfg,
		status:    StatusStopped,
		stopCh:    make(chan struct{}),
		rebuildCh: make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
}

// Status returns the current status snapshot.
func (m *ManagedProcess) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *ManagedProcess) setStatus(s Status) {
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
}

// Snapshot returns a copy of the externally observable state, suitable for
// the list RPC.
func (m *ManagedProcess) Snapshot() (status Status, pid, restarts int, startedAt time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status, m.pid, m.restarts, m.startedAt
}

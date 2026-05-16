// Package state persists the supervisor's app registry to a JSON file so
// that the daemon can resume where it left off after a restart or reboot.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/thesoftwaremasons/gopm/pkg/config"
)

// PersistedApp captures everything we need to recreate a managed app from
// disk. Status and PID are deliberately not persisted: a freshly-started
// daemon assumes its previous children are dead and reschedules them.
type PersistedApp struct {
	Config   config.AppConfig `json:"config"`
	Restarts int              `json:"restarts,omitempty"`
}

// State is the on-disk shape of the registry.
type State struct {
	Apps []PersistedApp `json:"apps"`
}

// Store is a thread-safe JSON file accessor.
type Store struct {
	mu   sync.Mutex
	path string
}

// DefaultPath returns the path used when no override is supplied
// (~/.gopm/state.json).
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, ".gopm", "state.json"), nil
}

// New returns a Store backed by path. The parent directory is created
// lazily on first Save.
func New(path string) *Store {
	return &Store{path: path}
}

// Path returns the on-disk file path.
func (s *Store) Path() string { return s.path }

// Save writes st to disk atomically: write to a temp file, fsync, rename.
// A nil receiver or an empty path is a no-op so callers can disable
// persistence by passing nil/"".
func (s *Store) Save(st *State) error {
	if s == nil || s.path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}

	body, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// Best-effort cleanup if rename fails before we get there.
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsync tmp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close tmp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return fmt.Errorf("rename tmp: %w", err)
	}
	return nil
}

// Load reads the state file. If the file does not exist (first-ever run),
// it returns an empty State and no error.
func (s *Store) Load() (*State, error) {
	if s == nil || s.path == "" {
		return &State{}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	body, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var st State
	if err := json.Unmarshal(body, &st); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &st, nil
}

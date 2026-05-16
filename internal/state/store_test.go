package state

import (
	"path/filepath"
	"testing"

	"github.com/thesoftwaremasons/gopm/pkg/config"
)

func TestStoreSaveLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	store := New(path)

	in := &State{Apps: []PersistedApp{
		{
			Config: config.AppConfig{Name: "api", Command: "go"},
		},
	}}
	if err := store.Save(in); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	out, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(out.Apps) != 1 || out.Apps[0].Config.Name != "api" {
		t.Fatalf("Load() = %#v", out)
	}
}

func TestLoadMissingFileReturnsEmptyState(t *testing.T) {
	store := New(filepath.Join(t.TempDir(), "missing", "state.json"))
	st, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(st.Apps) != 0 {
		t.Fatalf("Load() apps = %d, want 0", len(st.Apps))
	}
}

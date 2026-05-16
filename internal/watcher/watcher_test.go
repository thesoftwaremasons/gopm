package watcher

import (
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"github.com/thesoftwaremasons/gopm/pkg/config"
)

func TestShouldFireHonorsExtensionsAndIgnore(t *testing.T) {
	root := t.TempDir()
	ignored := filepath.Join(root, "vendor")

	w, err := New(config.WatchConfig{
		Enabled:    true,
		Extensions: []string{".go"},
		Ignore:     []string{ignored},
	}, root, make(chan struct{}, 1), nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer w.fsw.Close()

	if !w.shouldFire(fsnotify.Event{Name: filepath.Join(root, "main.go"), Op: fsnotify.Write}) {
		t.Fatal("expected .go write to fire")
	}
	if w.shouldFire(fsnotify.Event{Name: filepath.Join(root, "README.md"), Op: fsnotify.Write}) {
		t.Fatal("did not expect .md write to fire")
	}
	if w.shouldFire(fsnotify.Event{Name: filepath.Join(ignored, "main.go"), Op: fsnotify.Write}) {
		t.Fatal("did not expect ignored path to fire")
	}
}

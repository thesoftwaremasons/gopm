// Package watcher wraps fsnotify with the bits gopm needs: recursive
// directory tracking, extension filtering, ignore-prefix filtering,
// auto-watching of newly created subdirectories, and a single debounced
// "rebuild" notification.
package watcher

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/thesoftwaremasons/gopm/pkg/config"
)

// LogFunc receives diagnostic messages from the watcher.
type LogFunc func(string)

// Watcher monitors the directories listed in cfg and pushes a single signal
// to notify whenever a relevant file change is observed (after debouncing).
type Watcher struct {
	cfg    config.WatchConfig
	root   string // base for resolving relative dirs/ignore entries
	notify chan<- struct{}
	log    LogFunc

	fsw       *fsnotify.Watcher
	extSet    map[string]struct{}
	ignoreAbs []string
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// New constructs a Watcher. root is used to resolve relative entries in
// cfg.Dirs and cfg.Ignore. notify is the channel the supervise loop reads
// from; it should be buffered with capacity 1 so duplicate signals get
// coalesced.
func New(cfg config.WatchConfig, root string, notify chan<- struct{}, log LogFunc) (*Watcher, error) {
	if !cfg.Enabled {
		return nil, errors.New("watcher: not enabled")
	}
	if log == nil {
		log = func(string) {}
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	exts := make(map[string]struct{}, len(cfg.Extensions))
	for _, e := range cfg.Extensions {
		if e == "" {
			continue
		}
		if !strings.HasPrefix(e, ".") {
			e = "." + e
		}
		exts[strings.ToLower(e)] = struct{}{}
	}

	ignoreAbs := make([]string, 0, len(cfg.Ignore))
	for _, ig := range cfg.Ignore {
		ignoreAbs = append(ignoreAbs, absUnder(root, ig))
	}

	w := &Watcher{
		cfg:       cfg,
		root:      root,
		notify:    notify,
		log:       log,
		fsw:       fsw,
		extSet:    exts,
		ignoreAbs: ignoreAbs,
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
	return w, nil
}

// Start begins watching. It blocks until Close is called, so callers
// typically invoke it in its own goroutine.
func (w *Watcher) Start() {
	defer close(w.doneCh)

	totalAdded := 0
	for _, d := range w.cfg.Dirs {
		root := absUnder(w.root, d)
		added, err := w.addRecursive(root)
		totalAdded += added
		if err != nil {
			w.log("addRecursive " + root + ": " + err.Error())
		}
		w.log("watching " + root + " (" + itoa(added) + " dirs)")
	}
	if totalAdded == 0 {
		w.log("WARNING: no directories are being watched — check watch.dirs paths")
	}

	debounceMs := w.cfg.DebounceMs
	if debounceMs <= 0 {
		debounceMs = 300
	}
	debounce := time.Duration(debounceMs) * time.Millisecond

	var timer *time.Timer
	var timerC <-chan time.Time
	resetTimer := func() {
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer = time.NewTimer(debounce)
		timerC = timer.C
	}

	for {
		select {
		case <-w.stopCh:
			return
		case ev, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			// If a directory was just created, start watching it too.
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() && !w.isIgnored(ev.Name) {
					if _, err := w.addRecursive(ev.Name); err != nil {
						w.log("auto-add " + ev.Name + ": " + err.Error())
					}
				}
			}
			if !w.shouldFire(ev) {
				continue
			}
			resetTimer()
		case <-timerC:
			timer = nil
			timerC = nil
			select {
			case w.notify <- struct{}{}:
			default:
				// existing rebuild still pending; drop the signal
			}
		case err, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			w.log("fsnotify: " + err.Error())
		}
	}
}

// Close stops the watcher and waits for its goroutine to exit.
func (w *Watcher) Close() {
	select {
	case <-w.stopCh:
	default:
		close(w.stopCh)
	}
	_ = w.fsw.Close()
	<-w.doneCh
}

// addRecursive walks root and subscribes every (non-ignored) directory to
// fsnotify. It never aborts the walk on a single Add failure: if a
// directory can't be watched (permission, watch limit, anti-virus lock,
// already added) we log it and keep going, so a problem with one subdir
// doesn't silently disable the rest of the tree.
//
// Returns the number of directories successfully added and the last walk
// error encountered (the walk itself may fail if root doesn't exist).
func (w *Watcher) addRecursive(root string) (int, error) {
	added := 0
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip directories we can't read but keep walking siblings.
			if d != nil && d.IsDir() {
				w.log("walk " + path + ": " + err.Error())
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if w.isIgnored(path) {
			return fs.SkipDir
		}
		if addErr := w.fsw.Add(path); addErr != nil {
			// Log and continue — never let a single failed Add halt the walk.
			w.log("add watch " + path + ": " + addErr.Error())
			return nil
		}
		added++
		return nil
	})
	return added, walkErr
}

// itoa is a tiny helper so we don't have to import strconv in just one spot.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// shouldFire decides whether a given filesystem event should trigger a
// rebuild signal. It filters on extension and on the ignore-prefix list.
func (w *Watcher) shouldFire(ev fsnotify.Event) bool {
	// Only writes/creates/renames/removes of source files are interesting.
	if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}
	if w.isIgnored(ev.Name) {
		return false
	}
	if len(w.extSet) == 0 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(ev.Name))
	_, ok := w.extSet[ext]
	return ok
}

// isIgnored reports whether path lives under any of the configured ignore
// prefixes.
func (w *Watcher) isIgnored(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	abs = filepath.Clean(abs)
	for _, ig := range w.ignoreAbs {
		if abs == ig {
			return true
		}
		if strings.HasPrefix(abs, ig+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// absUnder resolves p against root if p is relative, then cleans it.
func absUnder(root, p string) string {
	if !filepath.IsAbs(p) {
		base := root
		if base == "" {
			if cwd, err := os.Getwd(); err == nil {
				base = cwd
			}
		}
		p = filepath.Join(base, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Clean(p)
	}
	return abs
}

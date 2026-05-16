// Package logger provides per-process log capture for gopm: a ring buffer
// of recent stdout/stderr lines (queried via `gopm logs`) and an optional
// append-mode file writer with rotation.
package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultCapacity is the default per-process ring buffer size in lines.
const DefaultCapacity = 1000

// LogEvent is a single log line event, used for live streaming.
type LogEvent struct {
	Name string
	Line string
}

// Logger is the registry of per-process log buffers.
type Logger struct {
	mu       sync.Mutex
	procs    map[string]*procLog
	capacity int

	// global subscribers receive events from all processes
	allSubsMu sync.Mutex
	allSubs   []chan LogEvent
}

// procLog is the per-process log state.
type procLog struct {
	name string

	bufMu sync.Mutex
	buf   *ringBuffer

	fileMu       sync.Mutex
	file         *os.File
	logPath      string
	logSize      int64 // atomic
	logMaxBytes  int64
	logMaxBackup int

	subsMu sync.Mutex
	subs   []chan LogEvent

	// back-reference to parent Logger for global subscriber dispatch
	parent *Logger
}

// New constructs a Logger with the given per-process buffer capacity.
// A capacity of 0 means use DefaultCapacity.
func New(capacity int) *Logger {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Logger{
		procs:    make(map[string]*procLog),
		capacity: capacity,
	}
}

// ensure returns the procLog for name, creating one if necessary.
func (l *Logger) ensure(name string) *procLog {
	l.mu.Lock()
	defer l.mu.Unlock()
	pl, ok := l.procs[name]
	if !ok {
		pl = &procLog{
			name:   name,
			buf:    newRingBuffer(l.capacity),
			parent: l,
		}
		l.procs[name] = pl
	}
	return pl
}

// SetLogFile configures (or re-configures) the on-disk log file for the
// named process. An empty path disables file output. The directory is
// created if it does not exist. maxSizeMB and maxBackups control rotation;
// 0 values disable rotation.
func (l *Logger) SetLogFile(name, path string, maxSizeMB, maxBackups int) error {
	pl := l.ensure(name)
	pl.fileMu.Lock()
	defer pl.fileMu.Unlock()

	if pl.file != nil {
		_ = pl.file.Close()
		pl.file = nil
	}
	if path == "" {
		pl.logPath = ""
		atomic.StoreInt64(&pl.logSize, 0)
		pl.logMaxBytes = 0
		pl.logMaxBackup = 0
		return nil
	}

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir log dir: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", path, err)
	}

	// Track current file size for rotation.
	info, _ := f.Stat()
	if info != nil {
		atomic.StoreInt64(&pl.logSize, info.Size())
	}

	pl.file = f
	pl.logPath = path
	if maxSizeMB > 0 {
		pl.logMaxBytes = int64(maxSizeMB) * 1024 * 1024
	}
	pl.logMaxBackup = maxBackups
	return nil
}

// Attach starts goroutines that read line-by-line from stdout and stderr,
// writing each line to the ring buffer (and log file, if configured).
// The goroutines exit when the readers EOF — typically when the child
// process terminates.
func (l *Logger) Attach(name string, stdout, stderr io.Reader) {
	pl := l.ensure(name)
	if stdout != nil {
		go pl.scan(stdout, "")
	}
	if stderr != nil {
		go pl.scan(stderr, "[stderr] ")
	}
}

// Write appends a single line to the named process's log. Useful for
// daemon-internal events (build started/failed, status changes) that
// should appear in `gopm logs` alongside child output.
func (l *Logger) Write(name, line string) {
	pl := l.ensure(name)
	pl.append(line)
}

// Tail returns the last n captured lines for the named process, oldest
// first. n <= 0 returns all buffered lines.
func (l *Logger) Tail(name string, n int) []string {
	l.mu.Lock()
	pl, ok := l.procs[name]
	l.mu.Unlock()
	if !ok {
		return nil
	}
	pl.bufMu.Lock()
	defer pl.bufMu.Unlock()
	entries := pl.buf.tail(n)
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.line
	}
	return out
}

// TailFiltered returns the last n lines for name, filtered by since time and
// grep pattern. since zero means no time filter; empty grep means no filter.
func (l *Logger) TailFiltered(name string, n int, since time.Time, grep string) []string {
	l.mu.Lock()
	pl, ok := l.procs[name]
	l.mu.Unlock()
	if !ok {
		return nil
	}
	pl.bufMu.Lock()
	defer pl.bufMu.Unlock()
	return pl.buf.tailFiltered(n, since, grep)
}

// Names returns the names of all processes that have log state.
func (l *Logger) Names() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, 0, len(l.procs))
	for name := range l.procs {
		names = append(names, name)
	}
	return names
}

// Subscribe creates and registers a channel that receives LogEvents for the
// named process. The caller must call Unsubscribe when done.
func (l *Logger) Subscribe(name string) chan LogEvent {
	pl := l.ensure(name)
	ch := make(chan LogEvent, 256)
	pl.subsMu.Lock()
	pl.subs = append(pl.subs, ch)
	pl.subsMu.Unlock()
	return ch
}

// SubscribeAll registers a channel that receives LogEvents from every process.
// The caller must call UnsubscribeAll when done.
func (l *Logger) SubscribeAll() chan LogEvent {
	ch := make(chan LogEvent, 512)
	l.allSubsMu.Lock()
	l.allSubs = append(l.allSubs, ch)
	l.allSubsMu.Unlock()
	return ch
}

// Unsubscribe removes ch from the named process's subscriber list.
func (l *Logger) Unsubscribe(name string, ch chan LogEvent) {
	l.mu.Lock()
	pl, ok := l.procs[name]
	l.mu.Unlock()
	if !ok {
		return
	}
	pl.subsMu.Lock()
	defer pl.subsMu.Unlock()
	for i, s := range pl.subs {
		if s == ch {
			pl.subs = append(pl.subs[:i], pl.subs[i+1:]...)
			return
		}
	}
}

// UnsubscribeAll removes ch from the global subscriber list.
func (l *Logger) UnsubscribeAll(ch chan LogEvent) {
	l.allSubsMu.Lock()
	defer l.allSubsMu.Unlock()
	for i, s := range l.allSubs {
		if s == ch {
			l.allSubs = append(l.allSubs[:i], l.allSubs[i+1:]...)
			return
		}
	}
}

// Detach closes the log file for the named process (if any) and drops
// its ring buffer. Used when an app is deleted from supervision.
func (l *Logger) Detach(name string) {
	l.mu.Lock()
	pl, ok := l.procs[name]
	if ok {
		delete(l.procs, name)
	}
	l.mu.Unlock()
	if !ok {
		return
	}
	pl.fileMu.Lock()
	if pl.file != nil {
		_ = pl.file.Close()
		pl.file = nil
	}
	pl.fileMu.Unlock()
}

// scan reads stdin line-by-line until EOF, prefixing each line with prefix
// before appending it to the ring buffer and (if configured) the file.
func (pl *procLog) scan(r io.Reader, prefix string) {
	sc := bufio.NewScanner(r)
	// Allow long log lines (default scanner buffer is 64KB).
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if prefix != "" {
			line = prefix + line
		}
		pl.append(line)
	}
}

// append writes a line to both the ring buffer and the file (if any),
// timestamping the file copy so on-disk logs are useful in isolation.
// It also fans out to any subscribers.
func (pl *procLog) append(line string) {
	now := time.Now()

	pl.bufMu.Lock()
	pl.buf.add(logEntry{line: line, ts: now})
	pl.bufMu.Unlock()

	pl.fileMu.Lock()
	if pl.file != nil {
		stamp := now.Format("2006-01-02 15:04:05.000")
		written, _ := fmt.Fprintf(pl.file, "%s %s\n", stamp, line)
		if written > 0 && pl.logMaxBytes > 0 {
			newSize := atomic.AddInt64(&pl.logSize, int64(written))
			if newSize >= pl.logMaxBytes {
				pl.rotate()
			}
		}
	}
	pl.fileMu.Unlock()

	// Fan out to per-process subscribers (non-blocking).
	ev := LogEvent{Name: pl.name, Line: line}
	pl.subsMu.Lock()
	for _, ch := range pl.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	pl.subsMu.Unlock()

	// Fan out to global subscribers.
	pl.parent.allSubsMu.Lock()
	for _, ch := range pl.parent.allSubs {
		select {
		case ch <- ev:
		default:
		}
	}
	pl.parent.allSubsMu.Unlock()
}

// rotate renames .log → .log.1, .log.1 → .log.2 etc., then opens a fresh file.
// Must be called with fileMu held.
func (pl *procLog) rotate() {
	if pl.file == nil || pl.logPath == "" {
		return
	}
	_ = pl.file.Close()
	pl.file = nil

	// Shift backups: .log.N-1 → .log.N
	if pl.logMaxBackup > 0 {
		for i := pl.logMaxBackup - 1; i >= 1; i-- {
			from := fmt.Sprintf("%s.%d", pl.logPath, i)
			to := fmt.Sprintf("%s.%d", pl.logPath, i+1)
			_ = os.Rename(from, to)
		}
		_ = os.Rename(pl.logPath, pl.logPath+".1")
	} else {
		// No backups — just truncate.
		_ = os.Remove(pl.logPath)
	}

	f, err := os.OpenFile(pl.logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return
	}
	pl.file = f
	atomic.StoreInt64(&pl.logSize, 0)
}

// logEntry holds a single buffered log line with its timestamp.
type logEntry struct {
	line string
	ts   time.Time
}

// ringBuffer is a fixed-size circular buffer of logEntry.
type ringBuffer struct {
	data []logEntry
	head int // index of next write
	size int // number of items currently held (<= cap)
	cap  int
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{
		data: make([]logEntry, capacity),
		cap:  capacity,
	}
}

func (r *ringBuffer) add(e logEntry) {
	r.data[r.head] = e
	r.head = (r.head + 1) % r.cap
	if r.size < r.cap {
		r.size++
	}
}

// tail returns the last n entries in chronological order. n <= 0 returns
// all buffered entries.
func (r *ringBuffer) tail(n int) []logEntry {
	if r.size == 0 {
		return nil
	}
	if n <= 0 || n > r.size {
		n = r.size
	}
	out := make([]logEntry, 0, n)
	start := (r.head - r.size + r.cap) % r.cap
	if n < r.size {
		start = (r.head - n + r.cap) % r.cap
	}
	for i := 0; i < n; i++ {
		out = append(out, r.data[(start+i)%r.cap])
	}
	return out
}

// tailFiltered returns last n lines filtered by since time and grep string.
func (r *ringBuffer) tailFiltered(n int, since time.Time, grep string) []string {
	all := r.tail(0) // get all entries
	var filtered []logEntry
	for _, e := range all {
		if !since.IsZero() && e.ts.Before(since) {
			continue
		}
		if grep != "" && !strings.Contains(e.line, grep) {
			continue
		}
		filtered = append(filtered, e)
	}
	// Apply n limit.
	if n > 0 && len(filtered) > n {
		filtered = filtered[len(filtered)-n:]
	}
	out := make([]string, len(filtered))
	for i, e := range filtered {
		out[i] = e.line
	}
	return out
}

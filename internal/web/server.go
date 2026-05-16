// Package web provides a lightweight HTTP dashboard for gopm. It runs in the
// CLI process and communicates with the daemon via IPC, so no daemon changes
// are needed. The dashboard serves a static HTML page plus JSON/SSE APIs.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/thesoftwaremasons/gopm/internal/ipc"
)

//go:embed static
var staticFiles embed.FS

// Server is the web dashboard HTTP server. It uses the IPC client to fetch
// data from the daemon for every request.
type Server struct {
	addr    string
	ipcAddr string
}

// New creates a web Server. addr is the HTTP listen address (e.g.
// "127.0.0.1:6120"). ipcAddr is the daemon IPC address.
func New(addr, ipcAddr string) *Server {
	return &Server{addr: addr, ipcAddr: ipcAddr}
}

// Serve starts the HTTP server and blocks until ctx is cancelled or an OS
// interrupt is received.
func (srv *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()

	// Serve the embedded static files under /.
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("web: embed sub: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	// JSON process list.
	mux.HandleFunc("/api/processes", srv.handleProcesses)

	// SSE log stream.
	mux.HandleFunc("/api/logs/", srv.handleLogStream)

	// Restart / stop actions.
	mux.HandleFunc("/api/", srv.handleAction)

	// Prometheus metrics.
	mux.HandleFunc("/metrics", srv.handleMetrics)

	httpSrv := &http.Server{
		Addr:    srv.addr,
		Handler: mux,
	}

	// Stop on ctx cancellation or SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		fmt.Printf("gopm web: listening on http://%s\n", srv.addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutCtx)
	case err := <-errCh:
		return err
	}
}

func (srv *Server) client() *ipc.Client {
	return ipc.NewClient(srv.ipcAddr)
}

func (srv *Server) handleProcesses(w http.ResponseWriter, r *http.Request) {
	resp, err := srv.client().Send(&ipc.Request{Command: ipc.CmdList})
	if err != nil || !resp.OK {
		http.Error(w, "daemon error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	_ = enc.Encode(resp.Processes)
}

// handleLogStream serves SSE (Server-Sent Events) for a process's log.
// Path: /api/logs/{name}/stream
func (srv *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	// Parse path.
	path := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	if !strings.HasSuffix(path, "/stream") {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimSuffix(path, "/stream")
	if name == "" {
		http.Error(w, "missing process name", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ctx := r.Context()

	// Use the IPC Stream client.
	req := &ipc.Request{
		Command: ipc.CmdLogStream,
		AppName: name,
		Follow:  true,
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = srv.client().Stream(req, func(ev ipc.LogEvent) bool {
			select {
			case <-ctx.Done():
				return false
			default:
			}
			line := strings.ReplaceAll(ev.Line, "\n", " ")
			fmt.Fprintf(w, "data: %s\n\n", line)
			flusher.Flush()
			return true
		})
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// handleAction dispatches POST /api/{name}/restart and /api/{name}/stop.
func (srv *Server) handleAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	// Path: /api/{name}/{action}
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	name, action := parts[0], parts[1]

	var cmd ipc.CommandType
	switch action {
	case "restart":
		cmd = ipc.CmdRestart
	case "stop":
		cmd = ipc.CmdStop
	default:
		http.NotFound(w, r)
		return
	}

	resp, err := srv.client().Send(&ipc.Request{Command: cmd, AppName: name})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if !resp.OK {
		http.Error(w, resp.Error, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": resp.Message})
}

// handleMetrics serves a minimal Prometheus text format metrics page.
func (srv *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	resp, err := srv.client().Send(&ipc.Request{Command: ipc.CmdList})
	if err != nil || !resp.OK {
		http.Error(w, "daemon error", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	fmt.Fprintf(w, "# HELP gopm_process_up 1 if process is online or ready\n")
	fmt.Fprintf(w, "# TYPE gopm_process_up gauge\n")
	for _, p := range resp.Processes {
		up := 0
		if p.Status == "online" || p.Status == "ready" {
			up = 1
		}
		fmt.Fprintf(w, "gopm_process_up{name=%q} %d\n", p.Name, up)
	}

	fmt.Fprintf(w, "# HELP gopm_process_restarts total restart count\n")
	fmt.Fprintf(w, "# TYPE gopm_process_restarts counter\n")
	for _, p := range resp.Processes {
		fmt.Fprintf(w, "gopm_process_restarts{name=%q} %d\n", p.Name, p.Restarts)
	}

	fmt.Fprintf(w, "# HELP gopm_process_memory_bytes resident set size in bytes\n")
	fmt.Fprintf(w, "# TYPE gopm_process_memory_bytes gauge\n")
	for _, p := range resp.Processes {
		fmt.Fprintf(w, "gopm_process_memory_bytes{name=%q} %d\n", p.Name, p.Memory)
	}

	fmt.Fprintf(w, "# HELP gopm_process_cpu_percent CPU usage percent\n")
	fmt.Fprintf(w, "# TYPE gopm_process_cpu_percent gauge\n")
	for _, p := range resp.Processes {
		fmt.Fprintf(w, "gopm_process_cpu_percent{name=%q} %.2f\n", p.Name, p.CPU)
	}
}

package api

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
	_ "github.com/thesoftwaremasons/gopm/internal/polybase/adapter/mongodb"
	_ "github.com/thesoftwaremasons/gopm/internal/polybase/adapter/postgres"
	"github.com/thesoftwaremasons/gopm/internal/polybase/crypto"
	"github.com/thesoftwaremasons/gopm/internal/polybase/metadata"
)

//go:generate sh -c "cd ../../web && npm run build"

//go:embed all:webdist
var webFS embed.FS

// Server is the Polybase HTTP server.
type Server struct {
	port     int
	store    *metadata.Store
	router   http.Handler
	mu       sync.RWMutex
	adapters map[string]adapter.Adapter
}

// NewServer initialises the server: loads/creates master key, opens SQLite store.
func NewServer(port int, dataDir string) (*Server, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, fmt.Errorf("polybase: mkdir data dir: %w", err)
	}
	masterKey, err := crypto.LoadOrCreateMasterKey(filepath.Join(dataDir, "master.key"))
	if err != nil {
		return nil, err
	}
	store, err := metadata.NewStore(filepath.Join(dataDir, "metadata.db"), masterKey)
	if err != nil {
		return nil, err
	}
	s := &Server{
		port:     port,
		store:    store,
		adapters: make(map[string]adapter.Adapter),
	}
	s.router = s.buildRouter()
	return s, nil
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	return http.ListenAndServe(fmt.Sprintf(":%d", s.port), s.router)
}

func (s *Server) buildRouter() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/connections", s.listConnections)
		r.Post("/connections", s.createConnection)
		r.Get("/connections/{id}", s.getConnection)
		r.Put("/connections/{id}", s.updateConnection)
		r.Delete("/connections/{id}", s.deleteConnection)
		r.Post("/connections/{id}/test", s.testConnection)
		r.Get("/connections/{id}/schemas", s.listSchemas)
		r.Get("/connections/{id}/browse", s.browse)
		r.Post("/connections/{id}/query", s.runQuery)
		r.Get("/engines", s.listEngines)
	})

	// Serve embedded React SPA — fallback unknown paths to index.html.
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		log.Printf("polybase: web UI not embedded (%v) — API only", err)
		return r
	}
	fileServer := http.FileServer(http.FS(sub))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		// Files with extensions are served directly; everything else → index.html.
		if ext := filepath.Ext(req.URL.Path); ext != "" && ext != ".html" {
			fileServer.ServeHTTP(w, req)
			return
		}
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})
	return r
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, dst interface{}) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// safeConn strips the password before sending to the client.
func safeConn(c metadata.StoredConnection) metadata.StoredConnection {
	c.Config.Password = ""
	return c
}

// getOrConnect returns a live adapter for the connection, creating one if needed.
func (s *Server) getOrConnect(ctx context.Context, id string) (adapter.Adapter, error) {
	s.mu.RLock()
	a, ok := s.adapters[id]
	s.mu.RUnlock()
	if ok {
		return a, nil
	}
	conn, err := s.store.GetConnection(ctx, id)
	if err != nil {
		return nil, err
	}
	a, err = adapter.New(conn.Engine)
	if err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.Connect(cctx, conn.Config); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.adapters[id] = a
	s.mu.Unlock()
	return a, nil
}

func (s *Server) evictAdapter(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if a, ok := s.adapters[id]; ok {
		_ = a.Close()
		delete(s.adapters, id)
	}
}

// ── handlers ──────────────────────────────────────────────────────────────────

func (s *Server) listEngines(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, adapter.Engines())
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := s.store.ListConnections(r.Context())
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	safe := make([]metadata.StoredConnection, len(conns))
	for i, c := range conns {
		safe[i] = safeConn(c)
	}
	if safe == nil {
		safe = []metadata.StoredConnection{}
	}
	writeJSON(w, 200, safe)
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string                   `json:"name"`
		Engine string                   `json:"engine"`
		Config adapter.ConnectionConfig `json:"config"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, 400, "invalid JSON: "+err.Error())
		return
	}
	if req.Name == "" || req.Engine == "" {
		writeError(w, 400, "name and engine are required")
		return
	}
	id := uuid.New().String()
	req.Config.ID = id
	req.Config.Name = req.Name
	req.Config.Engine = req.Engine
	conn := metadata.StoredConnection{
		ID:     id,
		Name:   req.Name,
		Engine: req.Engine,
		Config: req.Config,
	}
	if err := s.store.SaveConnection(r.Context(), conn); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 201, safeConn(conn))
}

func (s *Server) getConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	writeJSON(w, 200, safeConn(conn))
}

func (s *Server) updateConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	var req struct {
		Name   string                   `json:"name"`
		Config adapter.ConnectionConfig `json:"config"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	// Merge only non-zero fields so the client needn't re-send the password.
	cfg := &existing.Config
	if req.Config.Host != "" {
		cfg.Host = req.Config.Host
	}
	if req.Config.Port != 0 {
		cfg.Port = req.Config.Port
	}
	if req.Config.Database != "" {
		cfg.Database = req.Config.Database
	}
	if req.Config.Username != "" {
		cfg.Username = req.Config.Username
	}
	if req.Config.Password != "" {
		cfg.Password = req.Config.Password
	}
	cfg.ReadOnly = req.Config.ReadOnly
	if req.Config.SSLMode != "" {
		cfg.SSLMode = req.Config.SSLMode
	}

	s.evictAdapter(id)

	if err := s.store.SaveConnection(r.Context(), existing); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, safeConn(existing))
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.evictAdapter(id)
	if err := s.store.DeleteConnection(r.Context(), id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	conn, err := s.store.GetConnection(r.Context(), id)
	if err != nil {
		writeError(w, 404, err.Error())
		return
	}
	a, err := adapter.New(conn.Engine)
	if err != nil {
		writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := a.Connect(ctx, conn.Config); err != nil {
		writeJSON(w, 200, map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}
	_ = a.Close()
	writeJSON(w, 200, map[string]interface{}{"ok": true})
}

func (s *Server) listSchemas(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.getOrConnect(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	schemas, err := a.ListSchemas(ctx)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, schemas)
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	q := r.URL.Query()
	opts := adapter.BrowseOpts{
		Schema: q.Get("schema"),
		Table:  q.Get("table"),
	}
	if v, _ := strconv.Atoi(q.Get("offset")); v >= 0 {
		opts.Offset = v
	}
	if v, _ := strconv.Atoi(q.Get("limit")); v > 0 {
		opts.Limit = v
	}
	if opts.Limit <= 0 || opts.Limit > 1000 {
		opts.Limit = 100
	}

	a, err := s.getOrConnect(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rs, err := a.Browse(ctx, opts)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rs)
}

func (s *Server) runQuery(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Query    string `json:"query"`
		RowLimit int    `json:"row_limit"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if body.RowLimit <= 0 || body.RowLimit > 1000 {
		body.RowLimit = 1000
	}
	a, err := s.getOrConnect(r.Context(), id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	rs, err := a.Query(ctx, body.Query, body.RowLimit)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, rs)
}

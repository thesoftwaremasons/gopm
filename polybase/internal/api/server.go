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
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/thesoftwaremasons/polybase/internal/adapter"
	_ "github.com/thesoftwaremasons/polybase/internal/adapter/mongodb"
	_ "github.com/thesoftwaremasons/polybase/internal/adapter/postgres"
	"github.com/thesoftwaremasons/polybase/internal/crypto"
	"github.com/thesoftwaremasons/polybase/internal/metadata"
)

//go:generate sh -c "cd ../../web && npm run build && rm -rf ../internal/api/webdist && cp -r dist ../internal/api/webdist"

//go:embed all:webdist
var webFS embed.FS

// Server is the Polybase HTTP server.
type Server struct {
	port   int
	store  *metadata.Store
	router *chi.Mux
	// live adapters keyed by connection ID
	mu       sync.RWMutex
	adapters map[string]adapter.Adapter
}

func NewServer(port int, dataDir string) (*Server, error) {
	if err := mkdirP(dataDir); err != nil {
		return nil, err
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

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf(":%d", s.port)
	log.Printf("polybase: listening on %s", addr)
	return http.ListenAndServe(addr, s.router)
}

func (s *Server) buildRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// REST API
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
	})

	// Serve embedded React UI — SPA fallback to index.html
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		log.Printf("polybase: warning — web UI not embedded (%v), serving API only", err)
		return r
	}
	fileServer := http.FileServer(http.FS(sub))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		// If the path has an extension (css, js, png, etc.), serve the file directly.
		// Otherwise fall back to index.html for SPA routing.
		if filepath.Ext(req.URL.Path) != "" {
			fileServer.ServeHTTP(w, req)
			return
		}
		// Rewrite to index.html
		req.URL.Path = "/"
		fileServer.ServeHTTP(w, req)
	})

	return r
}

// ── helpers ──────────────────────────────────────────────────────────────────

func mkdirP(path string) error {
	return os.MkdirAll(path, 0700)
}

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

// safeConn returns a StoredConnection with Password masked for API responses.
func safeConn(c metadata.StoredConnection) metadata.StoredConnection {
	c.Config.Password = ""
	return c
}

func (s *Server) getAdapter(ctx context.Context, id string) (adapter.Adapter, error) {
	s.mu.RLock()
	a, ok := s.adapters[id]
	s.mu.RUnlock()
	if ok {
		return a, nil
	}
	// Not connected yet — load and connect.
	conn, err := s.store.GetConnection(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.connectAdapter(ctx, conn)
}

func (s *Server) connectAdapter(ctx context.Context, conn metadata.StoredConnection) (adapter.Adapter, error) {
	a, err := adapter.New(conn.Engine)
	if err != nil {
		return nil, err
	}
	ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := a.Connect(ctx2, conn.Config); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.adapters[conn.ID] = a
	s.mu.Unlock()
	return a, nil
}

// ── handlers ─────────────────────────────────────────────────────────────────

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
	writeJSON(w, 200, safe)
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string                  `json:"name"`
		Engine string                  `json:"engine"`
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
	req.Config.Name = req.Name
	req.Config.Engine = req.Engine
	id := newID()
	req.Config.ID = id
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
		Name   string                  `json:"name"`
		Config adapter.ConnectionConfig `json:"config"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Config.Host != "" {
		existing.Config.Host = req.Config.Host
	}
	if req.Config.Port != 0 {
		existing.Config.Port = req.Config.Port
	}
	if req.Config.Database != "" {
		existing.Config.Database = req.Config.Database
	}
	if req.Config.Username != "" {
		existing.Config.Username = req.Config.Username
	}
	if req.Config.Password != "" {
		existing.Config.Password = req.Config.Password
	}
	existing.Config.ReadOnly = req.Config.ReadOnly
	if req.Config.SSLMode != "" {
		existing.Config.SSLMode = req.Config.SSLMode
	}

	// Disconnect stale adapter so it reconnects on next use.
	s.mu.Lock()
	if a, ok := s.adapters[id]; ok {
		_ = a.Close()
		delete(s.adapters, id)
	}
	s.mu.Unlock()

	if err := s.store.SaveConnection(r.Context(), existing); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, safeConn(existing))
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.mu.Lock()
	if a, ok := s.adapters[id]; ok {
		_ = a.Close()
		delete(s.adapters, id)
	}
	s.mu.Unlock()
	if err := s.store.DeleteConnection(r.Context(), id); err != nil {
		writeError(w, 500, err.Error())
		return
	}
	w.WriteHeader(204)
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
	a, err := s.getAdapter(r.Context(), id)
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
	fmt.Sscanf(q.Get("offset"), "%d", &opts.Offset)
	fmt.Sscanf(q.Get("limit"), "%d", &opts.Limit)
	if opts.Limit <= 0 {
		opts.Limit = 100
	}
	if opts.Limit > 1000 {
		opts.Limit = 1000
	}

	a, err := s.getAdapter(r.Context(), id)
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
	a, err := s.getAdapter(r.Context(), id)
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

func newID() string {
	id, _ := uuid.NewRandom()
	return id.String()
}

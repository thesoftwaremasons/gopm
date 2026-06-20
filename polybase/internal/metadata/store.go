package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/thesoftwaremasons/polybase/internal/adapter"
	"github.com/thesoftwaremasons/polybase/internal/chart"
	"github.com/thesoftwaremasons/polybase/internal/crypto"
	"github.com/thesoftwaremasons/polybase/internal/join"
	_ "modernc.org/sqlite"
)

// StoredConnection is a connection saved in the metadata DB.
// The Password field inside Config is encrypted when stored, plaintext when loaded.
type StoredConnection struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Engine    string                  `json:"engine"`
	Config    adapter.ConnectionConfig `json:"config"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type Store struct {
	db  *sql.DB
	key []byte
}

func NewStore(path string, masterKey []byte) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("metadata: open: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("metadata: pragma: %w", err)
	}
	s := &Store{db: db, key: masterKey}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS connections (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			engine     TEXT NOT NULL,
			config_enc TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS join_definitions (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			definition TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS chart_definitions (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			definition TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	return err
}

// ── Join CRUD ─────────────────────────────────────────────────────────────────

func (s *Store) SaveJoin(ctx context.Context, j join.JoinDefinition) error {
	data, err := json.Marshal(j)
	if err != nil {
		return fmt.Errorf("metadata: marshal join: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO join_definitions(id,name,definition,created_at,updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, definition=excluded.definition, updated_at=excluded.updated_at`,
		j.ID, j.Name, string(data), now, now)
	return err
}

func (s *Store) ListJoins(ctx context.Context) ([]join.JoinDefinition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT definition FROM join_definitions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []join.JoinDefinition
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var j join.JoinDefinition
		if err := json.Unmarshal([]byte(raw), &j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) GetJoin(ctx context.Context, id string) (join.JoinDefinition, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT definition FROM join_definitions WHERE id=?`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return join.JoinDefinition{}, fmt.Errorf("join %q not found", id)
	}
	if err != nil {
		return join.JoinDefinition{}, err
	}
	var j join.JoinDefinition
	return j, json.Unmarshal([]byte(raw), &j)
}

func (s *Store) DeleteJoin(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM join_definitions WHERE id=?`, id)
	return err
}

func (s *Store) SaveConnection(ctx context.Context, conn StoredConnection) error {
	cfgCopy := conn.Config
	if cfgCopy.Password != "" {
		enc, err := crypto.EncryptString(s.key, cfgCopy.Password)
		if err != nil {
			return fmt.Errorf("metadata: encrypt password: %w", err)
		}
		cfgCopy.Password = enc
	}
	cfgCopy.ID = conn.ID
	data, err := json.Marshal(cfgCopy)
	if err != nil {
		return fmt.Errorf("metadata: marshal config: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO connections(id,name,engine,config_enc,created_at,updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, engine=excluded.engine,
			config_enc=excluded.config_enc, updated_at=excluded.updated_at`,
		conn.ID, conn.Name, conn.Engine, string(data), now, now)
	return err
}

func (s *Store) ListConnections(ctx context.Context) ([]StoredConnection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,engine,config_enc,created_at,updated_at FROM connections ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredConnection
	for rows.Next() {
		var c StoredConnection
		var cfgEnc, createdAt, updatedAt string
		if err := rows.Scan(&c.ID, &c.Name, &c.Engine, &cfgEnc, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		var cfg adapter.ConnectionConfig
		if err := json.Unmarshal([]byte(cfgEnc), &cfg); err != nil {
			return nil, err
		}
		if cfg.Password != "" {
			pt, err := crypto.DecryptString(s.key, cfg.Password)
			if err != nil {
				return nil, fmt.Errorf("metadata: decrypt password for %s: %w", c.ID, err)
			}
			cfg.Password = pt
		}
		c.Config = cfg
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetConnection(ctx context.Context, id string) (StoredConnection, error) {
	var c StoredConnection
	var cfgEnc, createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT id,name,engine,config_enc,created_at,updated_at FROM connections WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Engine, &cfgEnc, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("connection %q not found", id)
	}
	if err != nil {
		return c, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	var cfg adapter.ConnectionConfig
	if err := json.Unmarshal([]byte(cfgEnc), &cfg); err != nil {
		return c, err
	}
	if cfg.Password != "" {
		pt, err := crypto.DecryptString(s.key, cfg.Password)
		if err != nil {
			return c, err
		}
		cfg.Password = pt
	}
	c.Config = cfg
	return c, nil
}

func (s *Store) DeleteConnection(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id=?`, id)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ── Chart CRUD ────────────────────────────────────────────────────────────────

func (s *Store) SaveChart(ctx context.Context, c chart.Definition) error {
	data, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("metadata: marshal chart: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO chart_definitions(id,name,definition,created_at,updated_at)
		VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name, definition=excluded.definition, updated_at=excluded.updated_at`,
		c.ID, c.Name, string(data), now, now)
	return err
}

func (s *Store) ListCharts(ctx context.Context) ([]chart.Definition, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT definition FROM chart_definitions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chart.Definition
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var c chart.Definition
		if err := json.Unmarshal([]byte(raw), &c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) GetChart(ctx context.Context, id string) (chart.Definition, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT definition FROM chart_definitions WHERE id=?`, id).Scan(&raw)
	if err == sql.ErrNoRows {
		return chart.Definition{}, fmt.Errorf("chart %q not found", id)
	}
	if err != nil {
		return chart.Definition{}, err
	}
	var c chart.Definition
	return c, json.Unmarshal([]byte(raw), &c)
}

func (s *Store) DeleteChart(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM chart_definitions WHERE id=?`, id)
	return err
}

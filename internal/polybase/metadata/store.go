package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
	"github.com/thesoftwaremasons/gopm/internal/polybase/crypto"
	_ "modernc.org/sqlite"
)

// StoredConnection is a connection record in the metadata DB.
// Config.Password is plaintext in memory; it is encrypted before being written.
type StoredConnection struct {
	ID        string                   `json:"id"`
	Name      string                   `json:"name"`
	Engine    string                   `json:"engine"`
	Config    adapter.ConnectionConfig `json:"config"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// Store manages the Polybase metadata SQLite database.
type Store struct {
	db  *sql.DB
	key []byte
}

// NewStore opens (or creates) the SQLite database at path and runs migrations.
func NewStore(path string, masterKey []byte) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("metadata: open: %w", err)
	}
	s := &Store{db: db, key: masterKey}
	if err := s.migrate(); err != nil {
		_ = db.Close()
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
	`)
	return err
}

// SaveConnection upserts a connection. The password is encrypted before storage.
func (s *Store) SaveConnection(ctx context.Context, conn StoredConnection) error {
	cfgCopy := conn.Config
	if cfgCopy.Password != "" {
		enc, err := crypto.EncryptString(s.key, cfgCopy.Password)
		if err != nil {
			return fmt.Errorf("metadata: encrypt password: %w", err)
		}
		cfgCopy.Password = enc
	}
	data, err := json.Marshal(cfgCopy)
	if err != nil {
		return fmt.Errorf("metadata: marshal: %w", err)
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

// ListConnections returns all stored connections with decrypted passwords.
func (s *Store) ListConnections(ctx context.Context) ([]StoredConnection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,engine,config_enc,created_at,updated_at FROM connections ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StoredConnection
	for rows.Next() {
		c, err := s.scanRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetConnection returns a single connection by ID with decrypted password.
func (s *Store) GetConnection(ctx context.Context, id string) (StoredConnection, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id,name,engine,config_enc,created_at,updated_at FROM connections WHERE id=?`, id)
	c, err := s.scanRow(row.Scan)
	if err == sql.ErrNoRows {
		return StoredConnection{}, fmt.Errorf("connection %q not found", id)
	}
	return c, err
}

// DeleteConnection removes a connection by ID.
func (s *Store) DeleteConnection(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id=?`, id)
	return err
}

// Close closes the underlying database.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) scanRow(scan func(...interface{}) error) (StoredConnection, error) {
	var c StoredConnection
	var cfgEnc, createdAt, updatedAt string
	if err := scan(&c.ID, &c.Name, &c.Engine, &cfgEnc, &createdAt, &updatedAt); err != nil {
		return c, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	var cfg adapter.ConnectionConfig
	if err := json.Unmarshal([]byte(cfgEnc), &cfg); err != nil {
		return c, fmt.Errorf("metadata: unmarshal config: %w", err)
	}
	if cfg.Password != "" {
		pt, err := crypto.DecryptString(s.key, cfg.Password)
		if err != nil {
			return c, fmt.Errorf("metadata: decrypt password for %s: %w", c.ID, err)
		}
		cfg.Password = pt
	}
	c.Config = cfg
	return c, nil
}

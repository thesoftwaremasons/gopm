package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
	"github.com/thesoftwaremasons/gopm/internal/polybase/crypto"
)

// StoredConnection is a connection record saved in the metadata DB.
type StoredConnection struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Engine    string                 `json:"engine"`
	Config    adapter.ConnectionConfig `json:"config"` // password field is encrypted ciphertext
	CreatedAt time.Time              `json:"created_at"`
}

// Store wraps an SQLite database for Polybase metadata.
type Store struct {
	db  *sql.DB
	key []byte // master encryption key
}

// NewStore opens (or creates) the SQLite database at path.
func NewStore(path string, masterKey []byte) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("metadata: open db: %w", err)
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
			config_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

// SaveConnection inserts or replaces a connection record, encrypting the password.
func (s *Store) SaveConnection(ctx context.Context, conn StoredConnection) error {
	// Encrypt the password before storage.
	cfgCopy := conn.Config
	if cfgCopy.Password != "" {
		enc, err := crypto.EncryptString(s.key, cfgCopy.Password)
		if err != nil {
			return fmt.Errorf("metadata: encrypt password: %w", err)
		}
		cfgCopy.Password = enc
	}
	conn.Config = cfgCopy

	b, err := json.Marshal(conn.Config)
	if err != nil {
		return fmt.Errorf("metadata: marshal config: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO connections(id,name,engine,config_json,created_at)
		 VALUES(?,?,?,?,?)`,
		conn.ID, conn.Name, conn.Engine, string(b), conn.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// ListConnections returns all stored connections, decrypting passwords.
func (s *Store) ListConnections(ctx context.Context) ([]StoredConnection, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,engine,config_json,created_at FROM connections ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var conns []StoredConnection
	for rows.Next() {
		var c StoredConnection
		var configJSON, createdAt string
		if err := rows.Scan(&c.ID, &c.Name, &c.Engine, &configJSON, &createdAt); err != nil {
			return nil, err
		}
		c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		if err := json.Unmarshal([]byte(configJSON), &c.Config); err != nil {
			return nil, fmt.Errorf("metadata: unmarshal config: %w", err)
		}
		if c.Config.Password != "" {
			dec, err := crypto.DecryptString(s.key, c.Config.Password)
			if err == nil {
				c.Config.Password = dec
			}
		}
		conns = append(conns, c)
	}
	return conns, rows.Err()
}

// GetConnection returns a single connection by ID, decrypting the password.
func (s *Store) GetConnection(ctx context.Context, id string) (StoredConnection, error) {
	var c StoredConnection
	var configJSON, createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,engine,config_json,created_at FROM connections WHERE id=?`, id,
	).Scan(&c.ID, &c.Name, &c.Engine, &configJSON, &createdAt)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("metadata: connection %q not found", id)
	}
	if err != nil {
		return c, err
	}
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	if err := json.Unmarshal([]byte(configJSON), &c.Config); err != nil {
		return c, fmt.Errorf("metadata: unmarshal config: %w", err)
	}
	if c.Config.Password != "" {
		dec, err := crypto.DecryptString(s.key, c.Config.Password)
		if err == nil {
			c.Config.Password = dec
		}
	}
	return c, nil
}

// DeleteConnection removes a connection by ID.
func (s *Store) DeleteConnection(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM connections WHERE id=?`, id)
	return err
}

// SetSetting saves a key-value setting.
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO settings(key,value) VALUES(?,?)`, key, value)
	return err
}

// GetSetting retrieves a setting value.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

package adapter

import "context"

type ConnectionConfig struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Engine   string            `json:"engine"` // "postgres", "mongodb"
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Database string            `json:"database"`
	Username string            `json:"username"`
	Password string            `json:"password"` // plaintext in memory only, never persisted
	ReadOnly bool              `json:"read_only"`
	SSLMode  string            `json:"ssl_mode"` // Postgres: disable|require|verify-full
	Options  map[string]string `json:"options"`
}

type Column struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
}

type TableInfo struct {
	Name    string   `json:"name"`
	Schema  string   `json:"schema"`
	Type    string   `json:"type"` // "table","view","collection"
	Columns []Column `json:"columns,omitempty"`
}

type SchemaInfo struct {
	Name   string      `json:"name"`
	Tables []TableInfo `json:"tables"`
}

type BrowseOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"` // capped to 1000 server-side
}

type MultiGetOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Field  string `json:"field"`
}

type ResultSet struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
}

type Adapter interface {
	Connect(ctx context.Context, cfg ConnectionConfig) error
	ListSchemas(ctx context.Context) ([]SchemaInfo, error)
	Browse(ctx context.Context, opts BrowseOpts) (ResultSet, error)
	MultiGet(ctx context.Context, keys []string, opts MultiGetOpts) (ResultSet, error)
	// Query executes a read-only query. If the adapter's config has ReadOnly=true,
	// any DML/DDL keyword causes an error before sending to the DB.
	Query(ctx context.Context, query string, rowLimit int) (ResultSet, error)
	Close() error
}

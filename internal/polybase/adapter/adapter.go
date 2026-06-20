package adapter

import "context"

// ConnectionConfig holds configuration for a database connection.
type ConnectionConfig struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Engine   string            `json:"engine"` // "postgres", "mongodb"
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Database string            `json:"database"`
	Username string            `json:"username"`
	Password string            `json:"password"` // plaintext in memory only
	ReadOnly bool              `json:"read_only"`
	SSLMode  string            `json:"ssl_mode"`
	Options  map[string]string `json:"options"`
}

// Column describes a column/field.
type Column struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
}

// TableInfo holds metadata about a table/collection.
type TableInfo struct {
	Name    string   `json:"name"`
	Schema  string   `json:"schema"`
	Type    string   `json:"type"` // "table","view","collection"
	Columns []Column `json:"columns"`
}

// SchemaInfo holds metadata about a schema/database.
type SchemaInfo struct {
	Name   string      `json:"name"`
	Tables []TableInfo `json:"tables"`
}

// BrowseOpts specifies parameters for browsing a table.
type BrowseOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// MultiGetOpts specifies parameters for fetching rows by key.
type MultiGetOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Field  string `json:"field"`
}

// ResultSet is returned by Browse, Query, and MultiGet.
type ResultSet struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
}

// Adapter is the interface every database adapter must implement.
type Adapter interface {
	Connect(ctx context.Context, cfg ConnectionConfig) error
	ListSchemas(ctx context.Context) ([]SchemaInfo, error)
	Browse(ctx context.Context, opts BrowseOpts) (ResultSet, error)
	MultiGet(ctx context.Context, keys []string, opts MultiGetOpts) (ResultSet, error)
	Query(ctx context.Context, query string, rowLimit int) (ResultSet, error)
	Close() error
}

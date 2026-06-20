package adapter

import "context"

// ConnectionConfig holds the runtime (decrypted) configuration for a connection.
// Password is plaintext in memory only — never persisted unencrypted.
type ConnectionConfig struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Engine   string            `json:"engine"`
	Host     string            `json:"host"`
	Port     int               `json:"port"`
	Database string            `json:"database"`
	Username string            `json:"username"`
	Password string            `json:"password"`
	ReadOnly bool              `json:"read_only"`
	SSLMode  string            `json:"ssl_mode"`
	Options  map[string]string `json:"options,omitempty"`
}

// Column describes a single column or document field.
type Column struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
	Nullable bool   `json:"nullable"`
}

// TableInfo holds metadata about a table, view, or collection.
type TableInfo struct {
	Name    string   `json:"name"`
	Schema  string   `json:"schema"`
	Type    string   `json:"type"` // "table", "view", "collection"
	Columns []Column `json:"columns,omitempty"`
}

// SchemaInfo groups tables under a named schema or database.
type SchemaInfo struct {
	Name   string      `json:"name"`
	Tables []TableInfo `json:"tables"`
}

// BrowseOpts parameterises a paginated table/collection scan.
type BrowseOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// MultiGetOpts parameterises a batch key-value fetch (used by the join engine).
type MultiGetOpts struct {
	Schema string `json:"schema"`
	Table  string `json:"table"`
	Field  string `json:"field"`
}

// ResultSet is the universal query/browse result.
type ResultSet struct {
	Columns []string        `json:"columns"`
	Rows    [][]interface{} `json:"rows"`
	Total   int64           `json:"total"`
	HasMore bool            `json:"has_more"`
}

// Adapter is the interface every database engine must implement.
type Adapter interface {
	Connect(ctx context.Context, cfg ConnectionConfig) error
	ListSchemas(ctx context.Context) ([]SchemaInfo, error)
	Browse(ctx context.Context, opts BrowseOpts) (ResultSet, error)
	MultiGet(ctx context.Context, keys []string, opts MultiGetOpts) (ResultSet, error)
	// Query executes a read-only query. DML/DDL is blocked when cfg.ReadOnly is true.
	Query(ctx context.Context, query string, rowLimit int) (ResultSet, error)
	Close() error
}

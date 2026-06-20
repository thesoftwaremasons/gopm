package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
)

func init() {
	adapter.Register("postgres", func() adapter.Adapter { return &postgresAdapter{} })
}

// denyList contains write SQL verbs that are blocked in read-only mode.
var denyList = []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE", "GRANT", "REVOKE"}

type postgresAdapter struct {
	db  *sql.DB
	cfg adapter.ConnectionConfig
}

func (a *postgresAdapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	a.cfg = cfg

	dsn := buildDSN(cfg)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(5)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	a.db = db
	return nil
}

func buildDSN(cfg adapter.ConnectionConfig) string {
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "prefer"
	}
	return fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.Database, cfg.Username, cfg.Password, sslmode)
}

func (a *postgresAdapter) ListSchemas(ctx context.Context) ([]adapter.SchemaInfo, error) {
	schemaRows, err := a.db.QueryContext(ctx,
		`SELECT schema_name FROM information_schema.schemata
		 WHERE schema_name NOT IN ('pg_catalog','information_schema','pg_toast')
		 ORDER BY schema_name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list schemas: %w", err)
	}
	defer schemaRows.Close()

	var schemas []adapter.SchemaInfo
	for schemaRows.Next() {
		var name string
		if err := schemaRows.Scan(&name); err != nil {
			return nil, err
		}
		schemas = append(schemas, adapter.SchemaInfo{Name: name})
	}
	if err := schemaRows.Err(); err != nil {
		return nil, err
	}

	// Populate tables for each schema.
	for i := range schemas {
		tables, err := a.listTables(ctx, schemas[i].Name)
		if err != nil {
			return nil, err
		}
		schemas[i].Tables = tables
	}
	return schemas, nil
}

func (a *postgresAdapter) listTables(ctx context.Context, schema string) ([]adapter.TableInfo, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT table_name, table_type FROM information_schema.tables
		 WHERE table_schema = $1 ORDER BY table_name`, schema)
	if err != nil {
		return nil, fmt.Errorf("postgres: list tables: %w", err)
	}
	defer rows.Close()

	var tables []adapter.TableInfo
	for rows.Next() {
		var name, ttype string
		if err := rows.Scan(&name, &ttype); err != nil {
			return nil, err
		}
		kind := "table"
		if ttype == "VIEW" {
			kind = "view"
		}
		cols, _ := a.listColumns(ctx, schema, name)
		tables = append(tables, adapter.TableInfo{
			Name:    name,
			Schema:  schema,
			Type:    kind,
			Columns: cols,
		})
	}
	return tables, rows.Err()
}

func (a *postgresAdapter) listColumns(ctx context.Context, schema, table string) ([]adapter.Column, error) {
	rows, err := a.db.QueryContext(ctx,
		`SELECT column_name, data_type, is_nullable FROM information_schema.columns
		 WHERE table_schema = $1 AND table_name = $2 ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []adapter.Column
	for rows.Next() {
		var name, dtype, nullable string
		if err := rows.Scan(&name, &dtype, &nullable); err != nil {
			return nil, err
		}
		cols = append(cols, adapter.Column{
			Name:     name,
			DataType: dtype,
			Nullable: nullable == "YES",
		})
	}
	return cols, rows.Err()
}

func (a *postgresAdapter) Browse(ctx context.Context, opts adapter.BrowseOpts) (adapter.ResultSet, error) {
	if opts.Limit <= 0 || opts.Limit > 1000 {
		opts.Limit = 100
	}
	qualName := fmt.Sprintf(`"%s"."%s"`, opts.Schema, opts.Table)
	countRow := a.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", qualName))
	var total int64
	_ = countRow.Scan(&total)

	q := fmt.Sprintf(`SELECT * FROM %s LIMIT %d OFFSET %d`, qualName, opts.Limit, opts.Offset)
	return a.execQuery(ctx, q, total)
}

func (a *postgresAdapter) MultiGet(ctx context.Context, keys []string, opts adapter.MultiGetOpts) (adapter.ResultSet, error) {
	if len(keys) == 0 {
		return adapter.ResultSet{}, nil
	}
	placeholders := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = k
	}
	qualName := fmt.Sprintf(`"%s"."%s"`, opts.Schema, opts.Table)
	q := fmt.Sprintf(`SELECT * FROM %s WHERE "%s" IN (%s)`, qualName, opts.Field, strings.Join(placeholders, ","))
	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return adapter.ResultSet{}, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (a *postgresAdapter) Query(ctx context.Context, query string, rowLimit int) (adapter.ResultSet, error) {
	if a.cfg.ReadOnly {
		if err := checkReadOnly(query); err != nil {
			return adapter.ResultSet{}, err
		}
	}
	if rowLimit <= 0 || rowLimit > 1000 {
		rowLimit = 1000
	}
	// Wrap in a subquery to enforce limit.
	wrapped := fmt.Sprintf("SELECT * FROM (%s) _q LIMIT %d", query, rowLimit)
	return a.execQuery(ctx, wrapped, 0)
}

func (a *postgresAdapter) execQuery(ctx context.Context, q string, total int64) (adapter.ResultSet, error) {
	rows, err := a.db.QueryContext(ctx, q)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: query: %w", err)
	}
	defer rows.Close()
	rs, err := scanRows(rows)
	if err != nil {
		return rs, err
	}
	if total > 0 {
		rs.Total = total
	} else {
		rs.Total = int64(len(rs.Rows))
	}
	return rs, nil
}

func scanRows(rows *sql.Rows) (adapter.ResultSet, error) {
	cols, err := rows.Columns()
	if err != nil {
		return adapter.ResultSet{}, err
	}
	var result adapter.ResultSet
	result.Columns = cols

	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return result, err
		}
		// Convert []byte to string for JSON friendliness.
		row := make([]interface{}, len(vals))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				row[i] = string(b)
			} else {
				row[i] = v
			}
		}
		result.Rows = append(result.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	result.HasMore = false
	return result, nil
}

func checkReadOnly(query string) error {
	fields := strings.Fields(strings.ToUpper(query))
	if len(fields) == 0 {
		return nil
	}
	for _, verb := range denyList {
		if fields[0] == verb {
			return fmt.Errorf("write operation %q is not allowed on a read-only connection", fields[0])
		}
	}
	return nil
}

func (a *postgresAdapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

// Ensure stdlib is used (registers "pgx" driver).
var _ = stdlib.GetDefaultConfig

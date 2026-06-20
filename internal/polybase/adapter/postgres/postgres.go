package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	adapter.Register("postgres", func() adapter.Adapter { return &pgAdapter{} })
}

type pgAdapter struct {
	db  *sql.DB
	cfg adapter.ConnectionConfig
}

func (a *pgAdapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	port := cfg.Port
	if port == 0 {
		port = 5432
	}
	dsn := fmt.Sprintf("host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		cfg.Host, port, cfg.Database, cfg.Username, cfg.Password, sslmode)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("postgres: ping: %w", err)
	}
	a.db = db
	a.cfg = cfg
	return nil
}

func (a *pgAdapter) ListSchemas(ctx context.Context) ([]adapter.SchemaInfo, error) {
	rows, err := a.db.QueryContext(ctx, `
		SELECT table_schema, table_name, table_type
		FROM information_schema.tables
		WHERE table_schema NOT IN ('pg_catalog','information_schema')
		ORDER BY table_schema, table_name`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list schemas: %w", err)
	}
	defer rows.Close()

	schemaMap := map[string]*adapter.SchemaInfo{}
	var order []string
	for rows.Next() {
		var sname, tname, ttype string
		if err := rows.Scan(&sname, &tname, &ttype); err != nil {
			return nil, err
		}
		if _, ok := schemaMap[sname]; !ok {
			schemaMap[sname] = &adapter.SchemaInfo{Name: sname}
			order = append(order, sname)
		}
		typ := "table"
		if strings.EqualFold(ttype, "VIEW") {
			typ = "view"
		}
		schemaMap[sname].Tables = append(schemaMap[sname].Tables, adapter.TableInfo{
			Name: tname, Schema: sname, Type: typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]adapter.SchemaInfo, 0, len(order))
	for _, n := range order {
		result = append(result, *schemaMap[n])
	}
	return result, nil
}

func (a *pgAdapter) Browse(ctx context.Context, opts adapter.BrowseOpts) (adapter.ResultSet, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	schema := opts.Schema
	if schema == "" {
		schema = "public"
	}
	var total int64
	_ = a.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %q.%q`, schema, opts.Table)).Scan(&total)

	rows, err := a.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT * FROM %q.%q LIMIT %d OFFSET %d`, schema, opts.Table, limit, opts.Offset))
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: browse: %w", err)
	}
	defer rows.Close()
	return scanRows(rows, total, limit, opts.Offset)
}

func (a *pgAdapter) MultiGet(ctx context.Context, keys []string, opts adapter.MultiGetOpts) (adapter.ResultSet, error) {
	placeholders := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = k
	}
	q := fmt.Sprintf(`SELECT * FROM %q.%q WHERE %q IN (%s)`,
		opts.Schema, opts.Table, opts.Field, strings.Join(placeholders, ","))
	rows, err := a.db.QueryContext(ctx, q, args...)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: multiget: %w", err)
	}
	defer rows.Close()
	return scanRows(rows, int64(len(keys)), len(keys), 0)
}

var writePrefixes = []string{
	"INSERT", "UPDATE", "DELETE", "DROP", "CREATE",
	"ALTER", "TRUNCATE", "REPLACE", "GRANT", "REVOKE",
}

func (a *pgAdapter) Query(ctx context.Context, query string, rowLimit int) (adapter.ResultSet, error) {
	if a.cfg.ReadOnly {
		upper := strings.ToUpper(strings.TrimSpace(query))
		for _, p := range writePrefixes {
			if strings.HasPrefix(upper, p) {
				return adapter.ResultSet{}, fmt.Errorf("connection is read-only: %s not allowed", p)
			}
		}
	}
	if rowLimit <= 0 || rowLimit > 1000 {
		rowLimit = 1000
	}
	wrapped := fmt.Sprintf("SELECT * FROM (%s) AS _q LIMIT %d", query, rowLimit)
	rows, err := a.db.QueryContext(ctx, wrapped)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: query: %w", err)
	}
	defer rows.Close()
	return scanRows(rows, 0, rowLimit, 0)
}

func (a *pgAdapter) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func scanRows(rows *sql.Rows, total int64, limit, offset int) (adapter.ResultSet, error) {
	cols, err := rows.Columns()
	if err != nil {
		return adapter.ResultSet{}, err
	}
	var rs adapter.ResultSet
	rs.Columns = cols
	rs.Total = total
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return adapter.ResultSet{}, err
		}
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		rs.Rows = append(rs.Rows, vals)
	}
	rs.HasMore = total > 0 && total > int64(offset+limit)
	return rs, rows.Err()
}

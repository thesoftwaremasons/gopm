package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/thesoftwaremasons/polybase/internal/adapter"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	adapter.Register("postgres", func() adapter.Adapter { return &pgAdapter{} })
}

type pgAdapter struct {
	db  *sql.DB
	cfg adapter.ConnectionConfig
}

// identRe matches safe SQL identifiers: letters, digits, underscores, starting with letter/underscore.
var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validIdent(s string) bool {
	return identRe.MatchString(s)
}

func (a *pgAdapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:   "/" + cfg.Database,
	}
	if cfg.Username != "" || cfg.Password != "" {
		u.User = url.UserPassword(cfg.Username, cfg.Password)
	}
	u.RawQuery = url.Values{"sslmode": {sslmode}}.Encode()
	db, err := sql.Open("pgx", u.String())
	if err != nil {
		return fmt.Errorf("postgres: open: %w", err)
	}
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
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

	schemas := map[string]*adapter.SchemaInfo{}
	var order []string
	for rows.Next() {
		var schemaName, tableName, tableType string
		if err := rows.Scan(&schemaName, &tableName, &tableType); err != nil {
			return nil, err
		}
		if _, ok := schemas[schemaName]; !ok {
			schemas[schemaName] = &adapter.SchemaInfo{Name: schemaName}
			order = append(order, schemaName)
		}
		typ := "table"
		if strings.EqualFold(tableType, "VIEW") {
			typ = "view"
		}
		schemas[schemaName].Tables = append(schemas[schemaName].Tables, adapter.TableInfo{
			Name:   tableName,
			Schema: schemaName,
			Type:   typ,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list schemas: %w", err)
	}
	result := make([]adapter.SchemaInfo, 0, len(order))
	for _, name := range order {
		result = append(result, *schemas[name])
	}
	return result, nil
}

func (a *pgAdapter) Browse(ctx context.Context, opts adapter.BrowseOpts) (adapter.ResultSet, error) {
	limit := opts.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	qSchema := opts.Schema
	if qSchema == "" {
		qSchema = "public"
	}
	if !validIdent(qSchema) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid schema name %q", qSchema)
	}
	if !validIdent(opts.Table) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid table name %q", opts.Table)
	}

	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM %q.%q`, qSchema, opts.Table)
	var total int64
	_ = a.db.QueryRowContext(ctx, countQuery).Scan(&total)

	query := fmt.Sprintf(`SELECT * FROM %q.%q LIMIT %d OFFSET %d`, qSchema, opts.Table, limit, opts.Offset)
	rows, err := a.db.QueryContext(ctx, query)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("postgres: browse: %w", err)
	}
	defer rows.Close()
	return scanRows(rows, total, limit, opts.Offset)
}

func (a *pgAdapter) MultiGet(ctx context.Context, keys []string, opts adapter.MultiGetOpts) (adapter.ResultSet, error) {
	if !validIdent(opts.Schema) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid schema name %q", opts.Schema)
	}
	if !validIdent(opts.Table) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid table name %q", opts.Table)
	}
	if !validIdent(opts.Field) {
		return adapter.ResultSet{}, fmt.Errorf("postgres: invalid field name %q", opts.Field)
	}
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

var writePrefixes = []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE", "REPLACE", "GRANT", "REVOKE"}

// isWriteQuery detects write operations including CTEs like WITH ... DELETE/INSERT/UPDATE.
func isWriteQuery(query string) bool {
	// Strip single-line comments
	var cleaned strings.Builder
	for _, line := range strings.Split(query, "\n") {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		cleaned.WriteString(line)
		cleaned.WriteByte(' ')
	}
	q := cleaned.String()
	// Strip block comments /* ... */
	for {
		start := strings.Index(q, "/*")
		end := strings.Index(q, "*/")
		if start == -1 || end <= start {
			break
		}
		q = q[:start] + " " + q[end+2:]
	}
	upper := strings.ToUpper(strings.TrimSpace(q))
	for _, prefix := range writePrefixes {
		if upper == prefix || strings.HasPrefix(upper, prefix+" ") || strings.HasPrefix(upper, prefix+"\t") {
			return true
		}
	}
	// CTE containing a write statement: WITH ... (INSERT|UPDATE|DELETE|...)
	if strings.HasPrefix(upper, "WITH ") || strings.HasPrefix(upper, "WITH\t") {
		for _, kw := range []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE"} {
			if strings.Contains(upper, " "+kw+" ") || strings.Contains(upper, "\t"+kw+" ") {
				return true
			}
		}
	}
	return false
}

func (a *pgAdapter) Query(ctx context.Context, query string, rowLimit int) (adapter.ResultSet, error) {
	if a.cfg.ReadOnly && isWriteQuery(query) {
		return adapter.ResultSet{}, fmt.Errorf("connection is read-only: write operations not allowed")
	}
	if rowLimit <= 0 || rowLimit > 1000 {
		rowLimit = 1000
	}
	rows, err := a.db.QueryContext(ctx, query)
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
	var result adapter.ResultSet
	result.Columns = cols
	result.Total = total
	for rows.Next() {
		if limit > 0 && len(result.Rows) >= limit {
			result.HasMore = true
			break
		}
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
		result.Rows = append(result.Rows, vals)
	}
	if err := rows.Err(); err != nil {
		return adapter.ResultSet{}, err
	}
	// Compute HasMore: prefer COUNT(*) when available, otherwise infer from row count.
	if !result.HasMore {
		if total > 0 {
			result.HasMore = total > int64(offset+len(result.Rows))
		} else {
			result.HasMore = limit > 0 && len(result.Rows) == limit
		}
	}
	return result, nil
}

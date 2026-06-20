package mongodb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
)

func init() {
	adapter.Register("mongodb", func() adapter.Adapter { return &mongoAdapter{} })
}

var denyList = []string{"INSERT", "UPDATE", "DELETE", "DROP", "CREATE", "ALTER", "TRUNCATE"}

type mongoAdapter struct {
	client *mongo.Client
	cfg    adapter.ConnectionConfig
}

func (a *mongoAdapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	a.cfg = cfg

	uri := buildURI(cfg)
	opts := options.Client().ApplyURI(uri).SetTimeout(10 * time.Second)
	client, err := mongo.Connect(opts)
	if err != nil {
		return fmt.Errorf("mongodb: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return fmt.Errorf("mongodb: ping: %w", err)
	}
	a.client = client
	return nil
}

func buildURI(cfg adapter.ConnectionConfig) string {
	if u, ok := cfg.Options["uri"]; ok && u != "" {
		return u
	}
	port := cfg.Port
	if port == 0 {
		port = 27017
	}
	if cfg.Username != "" {
		return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
			cfg.Username, cfg.Password, cfg.Host, port, cfg.Database)
	}
	return fmt.Sprintf("mongodb://%s:%d", cfg.Host, port)
}

func (a *mongoAdapter) ListSchemas(ctx context.Context) ([]adapter.SchemaInfo, error) {
	dbNames, err := a.client.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("mongodb: list databases: %w", err)
	}

	var schemas []adapter.SchemaInfo
	for _, dbName := range dbNames {
		db := a.client.Database(dbName)
		colNames, err := db.ListCollectionNames(ctx, bson.D{})
		if err != nil {
			continue
		}
		var tables []adapter.TableInfo
		for _, col := range colNames {
			tables = append(tables, adapter.TableInfo{
				Name:   col,
				Schema: dbName,
				Type:   "collection",
			})
		}
		schemas = append(schemas, adapter.SchemaInfo{Name: dbName, Tables: tables})
	}
	return schemas, nil
}

func (a *mongoAdapter) Browse(ctx context.Context, opts adapter.BrowseOpts) (adapter.ResultSet, error) {
	if opts.Limit <= 0 || opts.Limit > 1000 {
		opts.Limit = 100
	}
	dbName := opts.Schema
	if dbName == "" {
		dbName = a.cfg.Database
	}
	col := a.client.Database(dbName).Collection(opts.Table)

	total, _ := col.CountDocuments(ctx, bson.D{})

	findOpts := options.Find().SetSkip(int64(opts.Offset)).SetLimit(int64(opts.Limit))
	cursor, err := col.Find(ctx, bson.D{}, findOpts)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: browse: %w", err)
	}
	defer cursor.Close(ctx)

	return cursorToResultSet(ctx, cursor, total)
}

func (a *mongoAdapter) MultiGet(ctx context.Context, keys []string, opts adapter.MultiGetOpts) (adapter.ResultSet, error) {
	dbName := opts.Schema
	if dbName == "" {
		dbName = a.cfg.Database
	}
	col := a.client.Database(dbName).Collection(opts.Table)

	keyIfaces := make([]interface{}, len(keys))
	for i, k := range keys {
		keyIfaces[i] = k
	}
	filter := bson.D{{Key: opts.Field, Value: bson.D{{Key: "$in", Value: keyIfaces}}}}
	cursor, err := col.Find(ctx, filter)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: multiget: %w", err)
	}
	defer cursor.Close(ctx)
	return cursorToResultSet(ctx, cursor, 0)
}

func (a *mongoAdapter) Query(ctx context.Context, query string, rowLimit int) (adapter.ResultSet, error) {
	if a.cfg.ReadOnly {
		if err := checkReadOnly(query); err != nil {
			return adapter.ResultSet{}, err
		}
	}
	if rowLimit <= 0 || rowLimit > 1000 {
		rowLimit = 1000
	}

	// Parse as a JSON find filter; fall back to empty filter.
	var filter bson.D
	q := strings.TrimSpace(query)
	if q != "" && q != "{}" {
		if err := bson.UnmarshalExtJSON([]byte(q), false, &filter); err != nil {
			// treat as an empty filter rather than hard error
			filter = bson.D{}
		}
	}

	col := a.client.Database(a.cfg.Database).Collection(a.cfg.Database)
	findOpts := options.Find().SetLimit(int64(rowLimit))
	cursor, err := col.Find(ctx, filter, findOpts)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: query: %w", err)
	}
	defer cursor.Close(ctx)
	return cursorToResultSet(ctx, cursor, 0)
}

func cursorToResultSet(ctx context.Context, cursor *mongo.Cursor, total int64) (adapter.ResultSet, error) {
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return adapter.ResultSet{}, err
	}

	// Gather all unique keys as columns.
	colSet := map[string]int{}
	colOrder := []string{}
	for _, doc := range docs {
		for k := range doc {
			if _, exists := colSet[k]; !exists {
				colSet[k] = len(colOrder)
				colOrder = append(colOrder, k)
			}
		}
	}

	rows := make([][]interface{}, len(docs))
	for i, doc := range docs {
		row := make([]interface{}, len(colOrder))
		for k, idx := range colSet {
			v := doc[k]
			// Serialise nested objects/arrays to JSON strings.
			switch v.(type) {
			case bson.M, bson.D, bson.A:
				b, _ := json.Marshal(v)
				row[idx] = string(b)
			default:
				row[idx] = fmt.Sprintf("%v", v)
			}
		}
		rows[i] = row
	}

	if total == 0 {
		total = int64(len(docs))
	}
	return adapter.ResultSet{
		Columns: colOrder,
		Rows:    rows,
		Total:   total,
		HasMore: total > int64(len(docs)),
	}, nil
}

func checkReadOnly(query string) error {
	fields := strings.Fields(strings.ToUpper(query))
	if len(fields) == 0 {
		return nil
	}
	for _, verb := range denyList {
		if fields[0] == verb {
			return fmt.Errorf("write operation %q not allowed on read-only connection", fields[0])
		}
	}
	return nil
}

func (a *mongoAdapter) Close() error {
	if a.client != nil {
		return a.client.Disconnect(context.Background())
	}
	return nil
}

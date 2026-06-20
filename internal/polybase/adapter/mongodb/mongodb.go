package mongodb

import (
	"context"
	"fmt"
	"strings"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func init() {
	adapter.Register("mongodb", func() adapter.Adapter { return &mgAdapter{} })
}

type mgAdapter struct {
	client *mongo.Client
	db     *mongo.Database
	cfg    adapter.ConnectionConfig
}

func (a *mgAdapter) Connect(ctx context.Context, cfg adapter.ConnectionConfig) error {
	port := cfg.Port
	if port == 0 {
		port = 27017
	}
	var uri string
	if cfg.Username != "" {
		uri = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
			cfg.Username, cfg.Password, cfg.Host, port, cfg.Database)
	} else {
		uri = fmt.Sprintf("mongodb://%s:%d/%s", cfg.Host, port, cfg.Database)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("mongodb: connect: %w", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return fmt.Errorf("mongodb: ping: %w", err)
	}
	a.client = client
	a.db = client.Database(cfg.Database)
	a.cfg = cfg
	return nil
}

func (a *mgAdapter) ListSchemas(ctx context.Context) ([]adapter.SchemaInfo, error) {
	names, err := a.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("mongodb: list collections: %w", err)
	}
	tables := make([]adapter.TableInfo, len(names))
	for i, n := range names {
		tables[i] = adapter.TableInfo{Name: n, Schema: a.cfg.Database, Type: "collection"}
	}
	return []adapter.SchemaInfo{{Name: a.cfg.Database, Tables: tables}}, nil
}

func (a *mgAdapter) Browse(ctx context.Context, opts adapter.BrowseOpts) (adapter.ResultSet, error) {
	limit := int64(opts.Limit)
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	coll := a.db.Collection(opts.Table)
	total, _ := coll.CountDocuments(ctx, bson.D{})
	cursor, err := coll.Find(ctx, bson.D{},
		options.Find().SetSkip(int64(opts.Offset)).SetLimit(limit))
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: browse: %w", err)
	}
	defer cursor.Close(ctx)
	return scanCursor(ctx, cursor, total, int(limit), opts.Offset)
}

func (a *mgAdapter) MultiGet(ctx context.Context, keys []string, opts adapter.MultiGetOpts) (adapter.ResultSet, error) {
	ids := make(bson.A, len(keys))
	for i, k := range keys {
		ids[i] = k
	}
	filter := bson.D{{Key: opts.Field, Value: bson.D{{Key: "$in", Value: ids}}}}
	cursor, err := a.db.Collection(opts.Table).Find(ctx, filter)
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: multiget: %w", err)
	}
	defer cursor.Close(ctx)
	return scanCursor(ctx, cursor, int64(len(keys)), len(keys), 0)
}

var writePrefixes = []string{"insert", "update", "delete", "drop", "create", "rename"}

func (a *mgAdapter) Query(ctx context.Context, query string, rowLimit int) (adapter.ResultSet, error) {
	if a.cfg.ReadOnly {
		lower := strings.ToLower(strings.TrimSpace(query))
		for _, p := range writePrefixes {
			if strings.HasPrefix(lower, p) {
				return adapter.ResultSet{}, fmt.Errorf("connection is read-only: %s not allowed", p)
			}
		}
	}
	if rowLimit <= 0 || rowLimit > 1000 {
		rowLimit = 1000
	}
	// Query format: first line = collection name, rest = JSON filter (optional).
	parts := strings.SplitN(strings.TrimSpace(query), "\n", 2)
	collName := strings.TrimSpace(parts[0])
	var filter bson.D
	if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
		if err := bson.UnmarshalExtJSON([]byte(strings.TrimSpace(parts[1])), true, &filter); err != nil {
			return adapter.ResultSet{}, fmt.Errorf("mongodb: parse filter: %w", err)
		}
	}
	cursor, err := a.db.Collection(collName).Find(ctx, filter,
		options.Find().SetLimit(int64(rowLimit)))
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("mongodb: query: %w", err)
	}
	defer cursor.Close(ctx)
	return scanCursor(ctx, cursor, 0, rowLimit, 0)
}

func (a *mgAdapter) Close() error {
	if a.client != nil {
		return a.client.Disconnect(context.Background())
	}
	return nil
}

func scanCursor(ctx context.Context, cursor *mongo.Cursor, total int64, limit, offset int) (adapter.ResultSet, error) {
	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return adapter.ResultSet{}, err
	}
	if len(docs) == 0 {
		return adapter.ResultSet{Columns: []string{}, Rows: [][]interface{}{}, Total: total}, nil
	}
	colIdx := map[string]int{}
	var cols []string
	for _, doc := range docs {
		for k := range doc {
			if _, seen := colIdx[k]; !seen {
				colIdx[k] = len(cols)
				cols = append(cols, k)
			}
		}
	}
	rows := make([][]interface{}, len(docs))
	for i, doc := range docs {
		row := make([]interface{}, len(cols))
		for k, v := range doc {
			row[colIdx[k]] = v
		}
		rows[i] = row
	}
	return adapter.ResultSet{
		Columns: cols,
		Rows:    rows,
		Total:   total,
		HasMore: total > 0 && total > int64(offset+limit),
	}, nil
}

// Package join provides the cross-engine join engine (Phase 2).
package join

import (
	"context"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
)

// ConnectionID identifies a saved connection.
type ConnectionID = string

// JoinDefinition specifies how to join two data sources on a shared key.
type JoinDefinition struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	SourceA   ConnectionID `json:"source_a"`
	SourceB   ConnectionID `json:"source_b"`
	KeyFieldA string       `json:"key_field_a"`
	KeyFieldB string       `json:"key_field_b"`
	TableA    string       `json:"table_a"`
	TableB    string       `json:"table_b"`
	SchemaA   string       `json:"schema_a"`
	SchemaB   string       `json:"schema_b"`
}

// Engine executes cross-source joins (Phase 2).
type Engine struct{}

// Execute fetches SourceA, extracts join keys, batch-fetches SourceB via
// MultiGet, then merges results in memory. Stub — Phase 2 implementation pending.
func (e *Engine) Execute(_ context.Context, _ JoinDefinition, _ map[ConnectionID]adapter.Adapter) (adapter.ResultSet, error) {
	return adapter.ResultSet{}, nil
}

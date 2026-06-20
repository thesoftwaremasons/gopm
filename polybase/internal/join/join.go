// Package join provides the cross-engine hash join engine (Phase 2).
package join

import (
	"context"
	"fmt"

	"github.com/thesoftwaremasons/polybase/internal/adapter"
)

// ConnectionID identifies a saved connection.
type ConnectionID = string

// JoinDefinition defines how to join two data sources.
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
	CreatedAt string       `json:"created_at,omitempty"`
	UpdatedAt string       `json:"updated_at,omitempty"`
}

// Engine executes cross-source joins.
type Engine struct{}

// Execute performs a left hash join:
//  1. Browse all rows from Source A (up to 1 000)
//  2. Extract join key values from column KeyFieldA
//  3. MultiGet matching rows from Source B on KeyFieldB
//  4. Merge: every A row + matched B columns (null when no match)
//
// Output columns are prefixed "a.<col>" and "b.<col>" to avoid name collisions.
func (e *Engine) Execute(ctx context.Context, def JoinDefinition, adapters map[ConnectionID]adapter.Adapter) (adapter.ResultSet, error) {
	aAdapter, ok := adapters[def.SourceA]
	if !ok {
		return adapter.ResultSet{}, fmt.Errorf("join: adapter for source_a %q not loaded", def.SourceA)
	}
	bAdapter, ok := adapters[def.SourceB]
	if !ok {
		return adapter.ResultSet{}, fmt.Errorf("join: adapter for source_b %q not loaded", def.SourceB)
	}

	// ── Step 1: fetch from A ──────────────────────────────────────────────────
	rsA, err := aAdapter.Browse(ctx, adapter.BrowseOpts{
		Schema: def.SchemaA,
		Table:  def.TableA,
		Limit:  1000,
	})
	if err != nil {
		return adapter.ResultSet{}, fmt.Errorf("join: fetch source_a: %w", err)
	}

	// ── Step 2: locate key column in A ───────────────────────────────────────
	keyIdxA := -1
	for i, col := range rsA.Columns {
		if col == def.KeyFieldA {
			keyIdxA = i
			break
		}
	}
	if keyIdxA == -1 {
		return adapter.ResultSet{}, fmt.Errorf("join: key field %q not found in source_a columns %v", def.KeyFieldA, rsA.Columns)
	}

	// ── Step 3: extract unique keys ──────────────────────────────────────────
	seen := map[string]bool{}
	keys := make([]string, 0, len(rsA.Rows))
	for _, row := range rsA.Rows {
		if row[keyIdxA] == nil {
			continue
		}
		k := fmt.Sprintf("%v", row[keyIdxA])
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	// ── Step 4: batch-fetch from B ───────────────────────────────────────────
	var rsB adapter.ResultSet
	if len(keys) > 0 {
		rsB, err = bAdapter.MultiGet(ctx, keys, adapter.MultiGetOpts{
			Schema: def.SchemaB,
			Table:  def.TableB,
			Field:  def.KeyFieldB,
		})
		if err != nil {
			return adapter.ResultSet{}, fmt.Errorf("join: fetch source_b: %w", err)
		}
	}

	// ── Step 5: build B lookup map (key → first matching row) ────────────────
	keyIdxB := -1
	for i, col := range rsB.Columns {
		if col == def.KeyFieldB {
			keyIdxB = i
			break
		}
	}
	bLookup := make(map[string][]interface{}, len(rsB.Rows))
	if keyIdxB >= 0 {
		for _, row := range rsB.Rows {
			if row[keyIdxB] == nil {
				continue
			}
			k := fmt.Sprintf("%v", row[keyIdxB])
			if _, exists := bLookup[k]; !exists {
				bLookup[k] = row
			}
		}
	}

	// ── Step 6: build merged column list ─────────────────────────────────────
	cols := make([]string, 0, len(rsA.Columns)+len(rsB.Columns))
	for _, c := range rsA.Columns {
		cols = append(cols, "a."+c)
	}
	for _, c := range rsB.Columns {
		cols = append(cols, "b."+c)
	}

	// ── Step 7: merge rows (left join semantics) ──────────────────────────────
	rows := make([][]interface{}, 0, len(rsA.Rows))
	for _, aRow := range rsA.Rows {
		merged := make([]interface{}, len(cols))
		copy(merged[:len(rsA.Columns)], aRow)

		if aRow[keyIdxA] != nil {
			k := fmt.Sprintf("%v", aRow[keyIdxA])
			if bRow, ok := bLookup[k]; ok {
				copy(merged[len(rsA.Columns):], bRow)
			}
		}
		rows = append(rows, merged)
	}

	return adapter.ResultSet{
		Columns: cols,
		Rows:    rows,
		Total:   int64(len(rows)),
		HasMore: rsA.HasMore,
	}, nil
}

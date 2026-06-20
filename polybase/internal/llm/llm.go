// Package llm provides the LLM provider interface (Phase 4).
package llm

import "context"

// SchemaContext is the schema metadata sent to the LLM — never row data.
type SchemaContext struct {
	Engine  string        `json:"engine"`
	Schemas []SchemaEntry `json:"schemas"`
}

type SchemaEntry struct {
	Name   string       `json:"name"`
	Tables []TableEntry `json:"tables"`
}

type TableEntry struct {
	Name    string        `json:"name"`
	Columns []ColumnEntry `json:"columns"`
}

type ColumnEntry struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// QueryIntent is the structured output from the LLM — never raw query text
// that executes directly. The core service translates this into parameterized queries.
type QueryIntent struct {
	Collection  string            `json:"collection"`
	Aggregation string            `json:"aggregation"` // "count","sum","avg","max","min"
	GroupBy     string            `json:"group_by"`
	Filters     map[string]string `json:"filters"`
	ChartType   string            `json:"chart_type"` // "bar","line","pie","table"
}

// Provider is the interface for LLM backends (Phase 4).
type Provider interface {
	GenerateQueryIntent(ctx context.Context, question string, schema SchemaContext) (QueryIntent, error)
}

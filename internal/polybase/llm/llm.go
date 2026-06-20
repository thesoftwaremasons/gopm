// Package llm provides the pluggable LLM provider interface (Phase 4).
package llm

import "context"

// SchemaContext is the schema metadata sent to the LLM.
// Row data is never included — only table/column names and types.
type SchemaContext struct {
	Engine  string        `json:"engine"`
	Schemas []SchemaEntry `json:"schemas"`
}

// SchemaEntry is one schema/database in the context.
type SchemaEntry struct {
	Name   string       `json:"name"`
	Tables []TableEntry `json:"tables"`
}

// TableEntry is one table/collection.
type TableEntry struct {
	Name    string        `json:"name"`
	Columns []ColumnEntry `json:"columns"`
}

// ColumnEntry is one column/field.
type ColumnEntry struct {
	Name     string `json:"name"`
	DataType string `json:"data_type"`
}

// QueryIntent is the structured output from the LLM. The core service
// translates this into parameterised adapter queries — LLM output never
// executes as raw query text.
type QueryIntent struct {
	Collection  string            `json:"collection"`
	Aggregation string            `json:"aggregation"` // count, sum, avg, max, min
	GroupBy     string            `json:"group_by"`
	Filters     map[string]string `json:"filters"`
	ChartType   string            `json:"chart_type"` // bar, line, pie, table
}

// Provider is the interface for LLM backends (Phase 4).
// Implementations: OpenRouter, OpenAI, Anthropic, Ollama.
type Provider interface {
	GenerateQueryIntent(ctx context.Context, question string, schema SchemaContext) (QueryIntent, error)
}

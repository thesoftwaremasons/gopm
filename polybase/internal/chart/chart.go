// Package chart defines saved BI chart configurations (Phase 3).
package chart

// Definition is a saved chart: a SQL query + column selections + chart type.
type Definition struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ConnectionID string `json:"connection_id"`
	SQL          string `json:"sql"`
	XColumn      string `json:"x_column"`
	YColumn      string `json:"y_column"`
	ChartType    string `json:"chart_type"` // "bar","line","area","pie"
	CreatedAt    string `json:"created_at,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
}

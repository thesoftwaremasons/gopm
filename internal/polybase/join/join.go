// Package join provides a cross-connection join engine (Phase 2 stub).
package join

import (
	"context"

	"github.com/thesoftwaremasons/gopm/internal/polybase/adapter"
)

// JoinType represents the type of join operation.
type JoinType string

const (
	JoinTypeInner JoinType = "inner"
	JoinTypeLeft  JoinType = "left"
	JoinTypeRight JoinType = "right"
	JoinTypeFull  JoinType = "full"
)

// JoinKey defines how two result sets are joined.
type JoinKey struct {
	LeftColumn  string `json:"left_column"`
	RightColumn string `json:"right_column"`
}

// JoinSpec describes a join operation between two result sets.
type JoinSpec struct {
	Left     adapter.ResultSet `json:"left"`
	Right    adapter.ResultSet `json:"right"`
	JoinType JoinType          `json:"join_type"`
	Keys     []JoinKey         `json:"keys"`
}

// Join executes an in-memory join of two result sets.
// Phase 2 stub — returns a not-implemented error.
func Join(_ context.Context, _ JoinSpec) (adapter.ResultSet, error) {
	// TODO(phase2): implement hash join.
	return adapter.ResultSet{}, nil
}

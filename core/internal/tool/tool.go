package tool

import "context"

// Tool is the interface every agent tool must implement.
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]any // JSON Schema
	Execute(ctx context.Context, params map[string]any) (string, error)
}

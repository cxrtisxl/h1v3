package tool

import (
	"context"
	"fmt"
	"sync"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Registry holds registered tools and dispatches execution.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Unregister removes a tool by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Has returns true if a tool with the given name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}

// Get returns a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// List returns the names of all registered tools.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Definitions returns all tools in OpenAI function-calling format.
func (r *Registry) Definitions() []protocol.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]protocol.ToolDefinition, 0, len(r.tools))
	for _, t := range r.tools {
		defs = append(defs, protocol.NewToolDefinition(
			t.Name(),
			t.Description(),
			t.Parameters(),
		))
	}
	return defs
}

// Execute runs the named tool with the given parameters.
// Returns the tool output as a string, or an error description.
func (r *Registry) Execute(ctx context.Context, name string, params map[string]any) (string, error) {
	r.mu.RLock()
	t, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("tool %q not found", name)
	}
	return t.Execute(ctx, params)
}

// Len returns the number of registered tools.
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

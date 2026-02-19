package tool

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/h1v3-io/h1v3/internal/memory"
)

// ReadMemoryTool reads a memory scope's content.
type ReadMemoryTool struct {
	Store *memory.Store
}

func (t *ReadMemoryTool) Name() string        { return "read_memory" }
func (t *ReadMemoryTool) Description() string { return "Read the content of a memory scope." }
func (t *ReadMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"scope"},
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"description": "Name of the memory scope (e.g. project, preferences, team).",
			},
		},
	}
}

func (t *ReadMemoryTool) Execute(_ context.Context, params map[string]any) (string, error) {
	scope, _ := params["scope"].(string)
	if scope == "" {
		return "", fmt.Errorf("scope is required")
	}
	content := t.Store.Get(scope)
	if content == "" {
		return fmt.Sprintf("Memory scope %q is empty or does not exist.", scope), nil
	}
	return content, nil
}

// WriteMemoryTool writes content to a memory scope.
type WriteMemoryTool struct {
	Store *memory.Store
}

func (t *WriteMemoryTool) Name() string        { return "write_memory" }
func (t *WriteMemoryTool) Description() string { return "Write content to a memory scope, replacing any existing content." }
func (t *WriteMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"scope", "content"},
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"description": "Name of the memory scope (e.g. project, preferences, team).",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "The content to store.",
			},
		},
	}
}

func (t *WriteMemoryTool) Execute(_ context.Context, params map[string]any) (string, error) {
	scope, _ := params["scope"].(string)
	if scope == "" {
		return "", fmt.Errorf("scope is required")
	}
	content, _ := params["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}
	if err := t.Store.Set(scope, content); err != nil {
		return "", fmt.Errorf("write_memory: %w", err)
	}
	return fmt.Sprintf("Memory scope %q updated (%d bytes).", scope, len(content)), nil
}

// ListMemoryTool lists all memory scopes with their content lengths.
type ListMemoryTool struct {
	Store *memory.Store
}

func (t *ListMemoryTool) Name() string        { return "list_memory" }
func (t *ListMemoryTool) Description() string { return "List all memory scopes with content lengths." }
func (t *ListMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListMemoryTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	scopes := t.Store.List()
	if len(scopes) == 0 {
		return "No memory scopes found.", nil
	}

	names := make([]string, 0, len(scopes))
	for name := range scopes {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		fmt.Fprintf(&b, "- %s (%d bytes)\n", name, len(scopes[name]))
	}
	return b.String(), nil
}

// DeleteMemoryTool removes a memory scope.
type DeleteMemoryTool struct {
	Store *memory.Store
}

func (t *DeleteMemoryTool) Name() string        { return "delete_memory" }
func (t *DeleteMemoryTool) Description() string { return "Delete a memory scope." }
func (t *DeleteMemoryTool) Parameters() map[string]any {
	return map[string]any{
		"type":     "object",
		"required": []string{"scope"},
		"properties": map[string]any{
			"scope": map[string]any{
				"type":        "string",
				"description": "Name of the memory scope to delete.",
			},
		},
	}
}

func (t *DeleteMemoryTool) Execute(_ context.Context, params map[string]any) (string, error) {
	scope, _ := params["scope"].(string)
	if scope == "" {
		return "", fmt.Errorf("scope is required")
	}
	if err := t.Store.Delete(scope); err != nil {
		return "", fmt.Errorf("delete_memory: %w", err)
	}
	return fmt.Sprintf("Memory scope %q deleted.", scope), nil
}

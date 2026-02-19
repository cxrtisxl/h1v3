package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const maxReadSize = 100 * 1024 // 100KB

// checkPath validates that path is under allowedDir (if set).
func checkPath(path, allowedDir string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	if allowedDir != "" {
		allowed, _ := filepath.Abs(allowedDir)
		if !strings.HasPrefix(abs, allowed+string(filepath.Separator)) && abs != allowed {
			return "", fmt.Errorf("path %q is outside allowed directory %q", abs, allowed)
		}
	}
	return abs, nil
}

func getString(params map[string]any, key string) string {
	v, _ := params[key].(string)
	return v
}

// --- ReadFile ---

type ReadFileTool struct{ AllowedDir string }

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string  { return "Read the contents of a file" }
func (t *ReadFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "File path to read"},
		},
		"required": []string{"path"},
	}
}

func (t *ReadFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, err := checkPath(getString(params, "path"), t.AllowedDir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read_file: %w", err)
	}
	if len(data) > maxReadSize {
		return string(data[:maxReadSize]) + "\n... [truncated]", nil
	}
	return string(data), nil
}

// --- WriteFile ---

type WriteFileTool struct{ AllowedDir string }

func (t *WriteFileTool) Name() string        { return "write_file" }
func (t *WriteFileTool) Description() string  { return "Write content to a file (creates parent directories if needed)" }
func (t *WriteFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path to write"},
			"content": map[string]any{"type": "string", "description": "Content to write"},
		},
		"required": []string{"path", "content"},
	}
}

func (t *WriteFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, err := checkPath(getString(params, "path"), t.AllowedDir)
	if err != nil {
		return "", err
	}
	content := getString(params, "content")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("write_file: create dirs: %w", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
}

// --- EditFile ---

type EditFileTool struct{ AllowedDir string }

func (t *EditFileTool) Name() string        { return "edit_file" }
func (t *EditFileTool) Description() string  { return "Replace old_text with new_text in a file (old_text must be a unique match)" }
func (t *EditFileTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":     map[string]any{"type": "string", "description": "File path to edit"},
			"old_text": map[string]any{"type": "string", "description": "Text to find (must be unique)"},
			"new_text": map[string]any{"type": "string", "description": "Replacement text"},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (t *EditFileTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, err := checkPath(getString(params, "path"), t.AllowedDir)
	if err != nil {
		return "", err
	}
	oldText := getString(params, "old_text")
	newText := getString(params, "new_text")

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	content := string(data)

	count := strings.Count(content, oldText)
	if count == 0 {
		return "", fmt.Errorf("edit_file: old_text not found in %s", path)
	}
	if count > 1 {
		return "", fmt.Errorf("edit_file: old_text matches %d times in %s (must be unique)", count, path)
	}

	result := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(path, []byte(result), 0o644); err != nil {
		return "", fmt.Errorf("edit_file: write: %w", err)
	}
	return fmt.Sprintf("Replaced text in %s", path), nil
}

// --- ListDir ---

type ListDirTool struct{ AllowedDir string }

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string  { return "List directory contents with file sizes" }
func (t *ListDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path to list"},
		},
		"required": []string{"path"},
	}
}

func (t *ListDirTool) Execute(_ context.Context, params map[string]any) (string, error) {
	path, err := checkPath(getString(params, "path"), t.AllowedDir)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", fmt.Errorf("list_dir: %w", err)
	}

	var b strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if e.IsDir() {
			fmt.Fprintf(&b, "%s/\n", e.Name())
		} else {
			fmt.Fprintf(&b, "%s  %d bytes\n", e.Name(), info.Size())
		}
	}
	return b.String(), nil
}

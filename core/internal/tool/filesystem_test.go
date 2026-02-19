package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0o644)

	tool := &ReadFileTool{AllowedDir: dir}
	result, err := tool.Execute(context.Background(), map[string]any{"path": path})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello world" {
		t.Errorf("expected 'hello world', got %q", result)
	}
}

func TestReadFile_OutsideAllowedDir(t *testing.T) {
	tool := &ReadFileTool{AllowedDir: "/tmp/safe"}
	_, err := tool.Execute(context.Background(), map[string]any{"path": "/etc/passwd"})
	if err == nil {
		t.Fatal("expected error for path outside allowed dir")
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.txt")

	tool := &WriteFileTool{AllowedDir: dir}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    path,
		"content": "data",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}

	data, _ := os.ReadFile(path)
	if string(data) != "data" {
		t.Errorf("expected 'data', got %q", string(data))
	}
}

func TestEditFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	os.WriteFile(path, []byte("foo bar baz"), 0o644)

	tool := &EditFileTool{AllowedDir: dir}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "bar",
		"new_text": "qux",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "foo qux baz" {
		t.Errorf("expected 'foo qux baz', got %q", string(data))
	}
}

func TestEditFile_NotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dup.txt")
	os.WriteFile(path, []byte("aaa aaa"), 0o644)

	tool := &EditFileTool{AllowedDir: dir}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "aaa",
		"new_text": "bbb",
	})
	if err == nil {
		t.Fatal("expected error for non-unique match")
	}
}

func TestEditFile_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.txt")
	os.WriteFile(path, []byte("hello"), 0o644)

	tool := &EditFileTool{AllowedDir: dir}
	_, err := tool.Execute(context.Background(), map[string]any{
		"path":     path,
		"old_text": "missing",
		"new_text": "x",
	})
	if err == nil {
		t.Fatal("expected error for text not found")
	}
}

func TestListDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)

	tool := &ListDirTool{AllowedDir: dir}
	result, err := tool.Execute(context.Background(), map[string]any{"path": dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty listing")
	}
}

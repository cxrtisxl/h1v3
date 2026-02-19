package tool

import (
	"context"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/internal/memory"
)

func newTestMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	return memory.NewStore(t.TempDir())
}

func TestReadMemory(t *testing.T) {
	store := newTestMemoryStore(t)
	store.Set("project", "my project notes")

	tool := &ReadMemoryTool{Store: store}
	got, err := tool.Execute(context.Background(), map[string]any{"scope": "project"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "my project notes" {
		t.Errorf("got %q", got)
	}
}

func TestReadMemory_Missing(t *testing.T) {
	store := newTestMemoryStore(t)
	tool := &ReadMemoryTool{Store: store}

	got, err := tool.Execute(context.Background(), map[string]any{"scope": "nonexistent"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "empty or does not exist") {
		t.Errorf("expected missing message, got %q", got)
	}
}

func TestReadMemory_NoScope(t *testing.T) {
	store := newTestMemoryStore(t)
	tool := &ReadMemoryTool{Store: store}

	_, err := tool.Execute(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error for missing scope")
	}
}

func TestWriteMemory(t *testing.T) {
	store := newTestMemoryStore(t)
	tool := &WriteMemoryTool{Store: store}

	got, err := tool.Execute(context.Background(), map[string]any{
		"scope":   "preferences",
		"content": "user likes dark mode",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "updated") {
		t.Errorf("expected confirmation, got %q", got)
	}

	// Verify stored
	if v := store.Get("preferences"); v != "user likes dark mode" {
		t.Errorf("stored = %q", v)
	}
}

func TestWriteMemory_NoContent(t *testing.T) {
	store := newTestMemoryStore(t)
	tool := &WriteMemoryTool{Store: store}

	_, err := tool.Execute(context.Background(), map[string]any{"scope": "test"})
	if err == nil {
		t.Fatal("expected error for missing content")
	}
}

func TestListMemory(t *testing.T) {
	store := newTestMemoryStore(t)
	store.Set("project", "abc")
	store.Set("team", "defgh")

	tool := &ListMemoryTool{Store: store}
	got, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(got, "project (3 bytes)") {
		t.Errorf("missing project entry, got %q", got)
	}
	if !strings.Contains(got, "team (5 bytes)") {
		t.Errorf("missing team entry, got %q", got)
	}
}

func TestListMemory_Empty(t *testing.T) {
	store := newTestMemoryStore(t)
	tool := &ListMemoryTool{Store: store}

	got, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "No memory scopes") {
		t.Errorf("expected empty message, got %q", got)
	}
}

func TestDeleteMemory(t *testing.T) {
	store := newTestMemoryStore(t)
	store.Set("temp", "data")

	tool := &DeleteMemoryTool{Store: store}
	got, err := tool.Execute(context.Background(), map[string]any{"scope": "temp"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.Contains(got, "deleted") {
		t.Errorf("expected confirmation, got %q", got)
	}
	if v := store.Get("temp"); v != "" {
		t.Errorf("scope still exists: %q", v)
	}
}

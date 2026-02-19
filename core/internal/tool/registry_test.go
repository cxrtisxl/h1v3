package tool

import (
	"context"
	"testing"
)

// stubTool is a minimal Tool for testing.
type stubTool struct {
	name   string
	result string
}

func (s *stubTool) Name() string                { return s.name }
func (s *stubTool) Description() string          { return "stub tool" }
func (s *stubTool) Parameters() map[string]any   { return map[string]any{"type": "object"} }
func (s *stubTool) Execute(_ context.Context, params map[string]any) (string, error) {
	return s.result, nil
}

func TestRegistry_RegisterAndExecute(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "echo", result: "hello"})

	if !reg.Has("echo") {
		t.Fatal("expected registry to have 'echo'")
	}
	if reg.Has("missing") {
		t.Fatal("expected registry to not have 'missing'")
	}
	if reg.Len() != 1 {
		t.Fatalf("expected len 1, got %d", reg.Len())
	}

	result, err := reg.Execute(context.Background(), "echo", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestRegistry_ExecuteUnknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Execute(context.Background(), "nope", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "a", result: ""})
	reg.Register(&stubTool{name: "b", result: ""})

	defs := reg.Definitions()
	if len(defs) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(defs))
	}
	// Check they're in OpenAI format
	for _, d := range defs {
		if d.Type != "function" {
			t.Errorf("expected type 'function', got %q", d.Type)
		}
		if d.Function.Name == "" {
			t.Error("expected non-empty function name")
		}
	}
}

func TestRegistry_Unregister(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "temp", result: ""})
	reg.Unregister("temp")
	if reg.Has("temp") {
		t.Fatal("expected tool to be unregistered")
	}
}

func TestRegistry_List(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&stubTool{name: "x", result: ""})
	reg.Register(&stubTool{name: "y", result: ""})

	names := reg.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

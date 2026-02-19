package agent

import (
	"context"
	"testing"

	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func newMockProviderSingle(content string) *mockProvider {
	return &mockProvider{
		responses: []*protocol.ChatResponse{{Content: content}},
	}
}

func TestSpawnSubAgent_FilterTools(t *testing.T) {
	parentTools := tool.NewRegistry()
	parentTools.Register(&tool.ReadFileTool{AllowedDir: "/tmp"})
	parentTools.Register(&tool.ExecTool{WorkDir: "/tmp"})

	parent := &Agent{
		Spec: protocol.AgentSpec{
			ID:   "coder",
			Role: "Developer",
		},
		Provider: newMockProviderSingle("done"),
		Tools:    parentTools,
	}

	sub := SpawnSubAgent(parent, "test task", "test-label")

	if sub.ParentID != "coder" {
		t.Errorf("ParentID = %q", sub.ParentID)
	}
	if sub.Label != "test-label" {
		t.Errorf("Label = %q", sub.Label)
	}
	if !sub.Tools.Has("read_file") {
		t.Error("sub-agent should have read_file")
	}
	if !sub.Tools.Has("exec") {
		t.Error("sub-agent should have exec")
	}
}

func TestSpawnSubAgent_ExcludesUnsafeTools(t *testing.T) {
	parentTools := tool.NewRegistry()
	parentTools.Register(&tool.ReadFileTool{AllowedDir: "/tmp"})
	parentTools.Register(&dummyTool{name: "create_ticket"})
	parentTools.Register(&dummyTool{name: "spawn_subagent"})

	parent := &Agent{
		Spec:     protocol.AgentSpec{ID: "coder"},
		Provider: newMockProviderSingle("ok"),
		Tools:    parentTools,
	}

	sub := SpawnSubAgent(parent, "task", "label")

	if sub.Tools.Has("create_ticket") {
		t.Error("sub-agent should NOT have create_ticket")
	}
	if sub.Tools.Has("spawn_subagent") {
		t.Error("sub-agent should NOT have spawn_subagent")
	}
	if !sub.Tools.Has("read_file") {
		t.Error("sub-agent should have read_file")
	}
}

func TestSubAgent_Run(t *testing.T) {
	parentTools := tool.NewRegistry()
	parent := &Agent{
		Spec:     protocol.AgentSpec{ID: "coder"},
		Provider: newMockProviderSingle("task complete"),
		Tools:    parentTools,
	}

	sub := SpawnSubAgent(parent, "do something", "worker")
	result, err := sub.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result != "task complete" {
		t.Errorf("result = %q", result)
	}
}

// dummyTool is a minimal tool for testing tool filtering.
type dummyTool struct {
	name string
}

func (d *dummyTool) Name() string                                                   { return d.name }
func (d *dummyTool) Description() string                                            { return "dummy" }
func (d *dummyTool) Parameters() map[string]any                                     { return nil }
func (d *dummyTool) Execute(_ context.Context, _ map[string]any) (string, error) { return "ok", nil }

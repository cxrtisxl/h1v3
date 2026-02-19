package registry

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/internal/agent"
	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func dummyAgentObj(id string) *agent.Agent {
	return &agent.Agent{
		Spec:  protocol.AgentSpec{ID: id, CoreInstructions: "test"},
		Tools: tool.NewRegistry(),
	}
}

func TestListAgentsTool_WithAgents(t *testing.T) {
	r := newTestRegistry(t)
	// Register agents
	specA, agA := dummyAgent("agent-a")
	specB, agB := dummyAgent("agent-b")
	r.RegisterAgent(specA, agA)
	r.RegisterAgent(specB, agB)

	tool := &ListAgentsTool{Reg: r}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "agent-a") || !strings.Contains(result, "agent-b") {
		t.Errorf("expected both agents in listing, got %q", result)
	}
}

func TestListAgentsTool_Empty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	store, _ := ticket.NewSQLiteStore(path)
	t.Cleanup(func() { store.DB().Close() })
	r := New(store, nil)

	tool := &ListAgentsTool{Reg: r}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "No agents") {
		t.Errorf("expected 'No agents', got %q", result)
	}
}

func TestCreateAgentTool_Success(t *testing.T) {
	r := newTestRegistry(t)

	factoryCalled := false
	factory := func(spec protocol.AgentSpec) error {
		factoryCalled = true
		// Simulate registering the agent
		ag := dummyAgentObj(spec.ID)
		return r.RegisterAgent(spec, ag)
	}

	tool := &CreateAgentTool{Reg: r, AgentID: "agent-a", Factory: factory}
	result, err := tool.Execute(context.Background(), map[string]any{
		"name":         "new-agent",
		"role":         "Assistant",
		"instructions": "You help with tasks.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !factoryCalled {
		t.Error("expected factory to be called")
	}
	if !strings.Contains(result, "new-agent") {
		t.Errorf("expected agent name in result, got %q", result)
	}

	// Verify agent exists
	if _, ok := r.GetAgent("new-agent"); !ok {
		t.Error("expected new agent to be registered")
	}

	// Verify creator tracking
	r.mu.RLock()
	creator := r.creators["new-agent"]
	r.mu.RUnlock()
	if creator != "agent-a" {
		t.Errorf("expected creator 'agent-a', got %q", creator)
	}
}

func TestCreateAgentTool_Duplicate(t *testing.T) {
	r := newTestRegistry(t)
	specB, agB := dummyAgent("agent-b")
	r.RegisterAgent(specB, agB)

	tool := &CreateAgentTool{Reg: r, AgentID: "agent-a"}
	_, err := tool.Execute(context.Background(), map[string]any{
		"name":         "agent-b", // already exists
		"role":         "Test",
		"instructions": "test",
	})
	if err == nil {
		t.Fatal("expected error for duplicate agent")
	}
}

func TestDestroyAgentTool_Success(t *testing.T) {
	r := newTestRegistry(t)

	// Simulate agent-a created new-agent
	spec := protocol.AgentSpec{ID: "new-agent", CoreInstructions: "test"}
	ag := dummyAgentObj("new-agent")
	r.RegisterAgent(spec, ag)
	r.mu.Lock()
	r.creators["new-agent"] = "agent-a"
	r.mu.Unlock()

	tool := &DestroyAgentTool{Reg: r, AgentID: "agent-a"}
	result, err := tool.Execute(context.Background(), map[string]any{"agent_id": "new-agent"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "destroyed") {
		t.Errorf("expected 'destroyed' in result, got %q", result)
	}

	if _, ok := r.GetAgent("new-agent"); ok {
		t.Error("expected agent to be deregistered")
	}
}

func TestDestroyAgentTool_WrongCreator(t *testing.T) {
	r := newTestRegistry(t)

	spec := protocol.AgentSpec{ID: "new-agent", CoreInstructions: "test"}
	ag := dummyAgentObj("new-agent")
	r.RegisterAgent(spec, ag)
	r.mu.Lock()
	r.creators["new-agent"] = "agent-a"
	r.mu.Unlock()

	// agent-b tries to destroy agent-a's creation
	tool := &DestroyAgentTool{Reg: r, AgentID: "agent-b"}
	_, err := tool.Execute(context.Background(), map[string]any{"agent_id": "new-agent"})
	if err == nil {
		t.Fatal("expected error for wrong creator")
	}
}

func TestDestroyAgentTool_NotFound(t *testing.T) {
	r := newTestRegistry(t)
	tool := &DestroyAgentTool{Reg: r, AgentID: "agent-a"}
	_, err := tool.Execute(context.Background(), map[string]any{"agent_id": "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

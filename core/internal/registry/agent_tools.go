package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// --- ListAgentsTool ---

type ListAgentsTool struct {
	Reg *Registry
}

func (t *ListAgentsTool) Name() string        { return "list_agents" }
func (t *ListAgentsTool) Description() string  { return "List all agents in the hive with their roles" }
func (t *ListAgentsTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *ListAgentsTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	t.Reg.mu.RLock()
	defer t.Reg.mu.RUnlock()

	if len(t.Reg.agents) == 0 {
		return "No agents registered.", nil
	}

	var b strings.Builder
	for id, h := range t.Reg.agents {
		role := h.Spec.Role
		if role == "" {
			role = "(no role)"
		}
		fmt.Fprintf(&b, "- **%s**: %s\n", id, role)
	}
	return b.String(), nil
}

// --- CreateAgentTool ---

// AgentFactory is a callback the Registry uses to construct a new Agent.
// This avoids importing the agent package directly in tools.
type AgentFactory func(spec protocol.AgentSpec) error

type CreateAgentTool struct {
	Reg      *Registry
	AgentID  string // creator's ID
	Factory  AgentFactory
}

func (t *CreateAgentTool) Name() string        { return "create_agent" }
func (t *CreateAgentTool) Description() string  { return "Create a new agent in the hive" }
func (t *CreateAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "description": "Unique agent ID"},
			"role":         map[string]any{"type": "string", "description": "Agent role description"},
			"instructions": map[string]any{"type": "string", "description": "Core instructions for the agent"},
			"tools":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tool names to enable"},
		},
		"required": []string{"name", "role", "instructions"},
	}
}

func (t *CreateAgentTool) Execute(_ context.Context, params map[string]any) (string, error) {
	name := getString(params, "name")
	role := getString(params, "role")
	instructions := getString(params, "instructions")
	tools := getStringSlice(params, "tools")

	if name == "" || role == "" || instructions == "" {
		return "", fmt.Errorf("create_agent: name, role, and instructions are required")
	}

	// Check if agent already exists
	if _, exists := t.Reg.GetAgent(name); exists {
		return "", fmt.Errorf("create_agent: agent %q already exists", name)
	}

	spec := protocol.AgentSpec{
		ID:               name,
		Role:             role,
		CoreInstructions: instructions,
		ToolsWhitelist:   tools,
	}

	if t.Factory != nil {
		if err := t.Factory(spec); err != nil {
			return "", fmt.Errorf("create_agent: %w", err)
		}
	}

	// Track who created this agent
	t.Reg.mu.Lock()
	if t.Reg.creators == nil {
		t.Reg.creators = make(map[string]string)
	}
	t.Reg.creators[name] = t.AgentID
	t.Reg.mu.Unlock()

	return fmt.Sprintf("Agent %q created (role: %s, creator: %s)", name, role, t.AgentID), nil
}

// --- DestroyAgentTool ---

type DestroyAgentTool struct {
	Reg     *Registry
	AgentID string // destroyer's ID
}

func (t *DestroyAgentTool) Name() string        { return "destroy_agent" }
func (t *DestroyAgentTool) Description() string  { return "Destroy an agent you created" }
func (t *DestroyAgentTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"agent_id": map[string]any{"type": "string", "description": "ID of the agent to destroy"},
		},
		"required": []string{"agent_id"},
	}
}

func (t *DestroyAgentTool) Execute(_ context.Context, params map[string]any) (string, error) {
	agentID := getString(params, "agent_id")
	if agentID == "" {
		return "", fmt.Errorf("destroy_agent: agent_id is required")
	}

	// Check permissions — only the creator can destroy
	t.Reg.mu.RLock()
	creator, tracked := t.Reg.creators[agentID]
	t.Reg.mu.RUnlock()

	if tracked && creator != t.AgentID {
		return "", fmt.Errorf("destroy_agent: agent %q was created by %q, not %q", agentID, creator, t.AgentID)
	}

	if err := t.Reg.DeregisterAgent(agentID); err != nil {
		return "", fmt.Errorf("destroy_agent: %w", err)
	}

	// Clean up creator tracking
	t.Reg.mu.Lock()
	delete(t.Reg.creators, agentID)
	t.Reg.mu.Unlock()

	return fmt.Sprintf("Agent %q destroyed", agentID), nil
}

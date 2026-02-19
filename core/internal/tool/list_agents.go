package tool

import (
	"context"
	"encoding/json"
)

// AgentInfo holds basic agent metadata for the discovery tool.
type AgentInfo struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// AgentLister provides agent discovery. Implemented by the registry adapter
// in cmd/h1v3d to break the import cycle.
type AgentLister interface {
	ListAgentInfo() []AgentInfo
}

// ListAgentsTool lets agents discover other agents in the hive.
type ListAgentsTool struct {
	Lister AgentLister
}

func (t *ListAgentsTool) Name() string { return "list_agents" }
func (t *ListAgentsTool) Description() string {
	return "List all agents in the hive with their IDs and roles."
}
func (t *ListAgentsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *ListAgentsTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	agents := t.Lister.ListAgentInfo()
	out, _ := json.MarshalIndent(agents, "", "  ")
	return string(out), nil
}

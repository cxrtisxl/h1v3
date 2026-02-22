package protocol

import "slices"

// AgentSpec defines a persistent agent's configuration.
type AgentSpec struct {
	ID               string            `json:"id"`
	Role             string            `json:"role"`
	Provider         string            `json:"provider,omitempty"`
	CoreInstructions string            `json:"core_instructions"`
	ScopedContexts   map[string]string `json:"scoped_contexts,omitempty"`
	ToolsWhitelist   []string          `json:"tools_whitelist,omitempty"`
	ToolsBlacklist   []string          `json:"tools_blacklist,omitempty"`
	Skills           []string          `json:"skills,omitempty"`
	Directory        string            `json:"directory"`
	WakeSchedule     string            `json:"wake_schedule,omitempty"`
}

// ToolAllowed reports whether the named tool is permitted for this agent.
// If a whitelist is set, only listed tools are allowed (blacklist is ignored).
// If only a blacklist is set, all tools except listed ones are allowed.
// If neither is set, all tools are allowed.
func (s AgentSpec) ToolAllowed(name string) bool {
	if len(s.ToolsWhitelist) > 0 {
		return slices.Contains(s.ToolsWhitelist, name)
	}
	if len(s.ToolsBlacklist) > 0 {
		return !slices.Contains(s.ToolsBlacklist, name)
	}
	return true
}

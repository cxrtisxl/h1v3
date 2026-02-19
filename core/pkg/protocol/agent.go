package protocol

// AgentSpec defines a persistent agent's configuration.
type AgentSpec struct {
	ID               string            `json:"id"`
	Role             string            `json:"role"`
	Provider         string            `json:"provider,omitempty"`
	CoreInstructions string            `json:"core_instructions"`
	ScopedContexts   map[string]string `json:"scoped_contexts,omitempty"`
	Tools            []string          `json:"tools,omitempty"`
	Skills           []string          `json:"skills,omitempty"`
	Directory        string            `json:"directory"`
	WakeSchedule     string            `json:"wake_schedule,omitempty"`
}

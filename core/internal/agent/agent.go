package agent

import (
	"log/slog"

	"github.com/h1v3-io/h1v3/internal/memory"
	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

const defaultMaxIterations = 20

// Agent is a single AI agent with its own spec, provider, and tools.
type Agent struct {
	Spec          protocol.AgentSpec
	Provider      provider.Provider
	Tools         *tool.Registry
	Logger        *slog.Logger
	MaxIterations int
	Memory         *memory.Store // optional, injected at startup
	SkillDirs      []string      // parent dirs (scanned as {dir}/skills/), reloaded each prompt
	ExtraSkillDirs []string      // direct skill dirs (scanned as-is), from skill_paths config
}

// New creates a new Agent with sensible defaults.
func New(spec protocol.AgentSpec, prov provider.Provider, tools *tool.Registry) *Agent {
	return &Agent{
		Spec:          spec,
		Provider:      prov,
		Tools:         tools,
		Logger:        slog.Default(),
		MaxIterations: defaultMaxIterations,
	}
}

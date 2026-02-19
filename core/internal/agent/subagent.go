package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

const subAgentMaxIterations = 15

// SubAgent is an ephemeral one-shot worker. It runs a task, returns a result,
// and then dies. No memory, no persistence.
type SubAgent struct {
	ParentID string
	Label    string
	Task     string
	Provider provider.Provider
	Tools    *tool.Registry
	Logger   *slog.Logger
}

// SpawnSubAgent creates a sub-agent from a parent agent.
// It copies a subset of the parent's tools (excludes ticket and spawn tools to prevent recursion).
func SpawnSubAgent(parent *Agent, task, label string) *SubAgent {
	// Copy only safe tools (filesystem, shell, web, memory — no ticket/agent/spawn tools)
	subTools := tool.NewRegistry()
	safeTools := []string{
		"read_file", "write_file", "edit_file", "list_dir",
		"exec", "web_search", "web_fetch",
		"read_memory", "write_memory", "list_memory", "delete_memory",
	}
	for _, name := range safeTools {
		if t, ok := parent.Tools.Get(name); ok {
			subTools.Register(t)
		}
	}

	logger := parent.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &SubAgent{
		ParentID: parent.Spec.ID,
		Label:    label,
		Task:     task,
		Provider: parent.Provider,
		Tools:    subTools,
		Logger:   logger.With("subagent", label),
	}
}

// Run executes the sub-agent's task and returns the result.
func (s *SubAgent) Run(ctx context.Context) (string, error) {
	s.Logger.Info("sub-agent starting", "task", truncateStr(s.Task, 80))

	ag := &Agent{
		Spec: protocol.AgentSpec{
			ID:               fmt.Sprintf("%s/sub/%s", s.ParentID, s.Label),
			Role:             "Sub-agent worker",
			CoreInstructions: fmt.Sprintf("You are a focused sub-agent. Complete this task concisely and return the result:\n\n%s", s.Task),
		},
		Provider:      s.Provider,
		Tools:         s.Tools,
		Logger:        s.Logger,
		MaxIterations: subAgentMaxIterations,
	}

	result, err := ag.Run(ctx, s.Task)
	if err != nil {
		s.Logger.Error("sub-agent failed", "error", err)
		return "", err
	}

	s.Logger.Info("sub-agent completed", "result_len", len(result))
	return result, nil
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

package agent

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/internal/memory"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func TestBuildSystemPrompt_Basic(t *testing.T) {
	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "coder",
			Role:             "Software Engineer",
			CoreInstructions: "You write Go code.",
		},
		Tools:  reg,
		Logger: slog.Default(),
	}

	prompt := a.BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "# Agent: coder") {
		t.Error("expected agent ID in prompt")
	}
	if !strings.Contains(prompt, "Role: Software Engineer") {
		t.Error("expected role in prompt")
	}
	if !strings.Contains(prompt, "You write Go code.") {
		t.Error("expected core instructions in prompt")
	}
	if !strings.Contains(prompt, "# Current Time") {
		t.Error("expected current time section")
	}
	if !strings.Contains(prompt, "# Rules") {
		t.Error("expected rules section")
	}
}

func TestBuildSystemPrompt_WithScopedContexts(t *testing.T) {
	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "agent1",
			CoreInstructions: "test",
			ScopedContexts: map[string]string{
				"memory":  "User prefers dark mode.",
				"project": "Working on h1v3.",
			},
		},
		Tools:  reg,
		Logger: slog.Default(),
	}

	prompt := a.BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "# Context") {
		t.Error("expected context section")
	}
	if !strings.Contains(prompt, "## memory") {
		t.Error("expected memory scope")
	}
	if !strings.Contains(prompt, "User prefers dark mode.") {
		t.Error("expected memory content")
	}
}

func TestBuildSystemPrompt_WithTicket(t *testing.T) {
	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "agent1",
			CoreInstructions: "test",
		},
		Tools:  reg,
		Logger: slog.Default(),
	}

	ticket := &protocol.Ticket{
		ID:     "t-001",
		Title:  "Fix the bug",
		Status: protocol.TicketOpen,
		Messages: []protocol.Message{
			{Content: "msg1"},
			{Content: "msg2"},
		},
	}

	prompt := a.BuildSystemPrompt(ticket)

	if !strings.Contains(prompt, "# Current Ticket") {
		t.Error("expected ticket section")
	}
	if !strings.Contains(prompt, "ID: t-001") {
		t.Error("expected ticket ID")
	}
	if !strings.Contains(prompt, "Title: Fix the bug") {
		t.Error("expected ticket title")
	}
	if !strings.Contains(prompt, "Messages: 2") {
		t.Error("expected message count")
	}
}

func TestBuildSystemPrompt_WithTools(t *testing.T) {
	reg := tool.NewRegistry()
	reg.Register(&echoTool{})

	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "agent1",
			CoreInstructions: "test",
		},
		Tools:  reg,
		Logger: slog.Default(),
	}

	prompt := a.BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "# Available Tools") {
		t.Error("expected tools section")
	}
	if !strings.Contains(prompt, "**echo**") {
		t.Error("expected echo tool in listing")
	}
}

func TestBuildSystemPrompt_WithMemory(t *testing.T) {
	dir := t.TempDir()
	mem := memory.NewStore(dir)
	mem.Set("identity", "My name is Alex.")
	mem.Set("preferences", "User prefers concise answers.")

	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "agent1",
			CoreInstructions: "test",
		},
		Tools:  reg,
		Logger: slog.Default(),
		Memory: mem,
	}

	prompt := a.BuildSystemPrompt(nil)

	if !strings.Contains(prompt, "# Memory") {
		t.Error("expected memory section")
	}
	if !strings.Contains(prompt, "## identity") {
		t.Error("expected identity scope")
	}
	if !strings.Contains(prompt, "My name is Alex.") {
		t.Error("expected identity content")
	}
	if !strings.Contains(prompt, "## preferences") {
		t.Error("expected preferences scope")
	}
	if !strings.Contains(prompt, "User prefers concise answers.") {
		t.Error("expected preferences content")
	}

	// Verify deterministic order (identity before preferences)
	idxIdentity := strings.Index(prompt, "## identity")
	idxPreferences := strings.Index(prompt, "## preferences")
	if idxIdentity > idxPreferences {
		t.Error("expected scopes in alphabetical order (identity before preferences)")
	}
}

func TestBuildSystemPrompt_WithMemoryEmpty(t *testing.T) {
	dir := t.TempDir()
	mem := memory.NewStore(dir)

	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "agent1",
			CoreInstructions: "test",
		},
		Tools:  reg,
		Logger: slog.Default(),
		Memory: mem,
	}

	prompt := a.BuildSystemPrompt(nil)

	if strings.Contains(prompt, "# Memory") {
		t.Error("should not have memory section when store is empty")
	}
}

func TestBuildSystemPrompt_NoTicketNoContexts(t *testing.T) {
	reg := tool.NewRegistry()
	a := &Agent{
		Spec: protocol.AgentSpec{
			ID:               "minimal",
			CoreInstructions: "minimal agent",
		},
		Tools:  reg,
		Logger: slog.Default(),
	}

	prompt := a.BuildSystemPrompt(nil)

	// Should NOT contain context or ticket sections
	if strings.Contains(prompt, "# Context") {
		t.Error("should not have context section when no scoped contexts")
	}
	if strings.Contains(prompt, "# Current Ticket") {
		t.Error("should not have ticket section when no ticket")
	}
	if strings.Contains(prompt, "# Available Tools") {
		t.Error("should not have tools section when no tools registered")
	}
}

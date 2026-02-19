package agent

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// mockProvider is a test provider that returns a sequence of responses.
type mockProvider struct {
	responses []*protocol.ChatResponse
	callIdx   int
	calls     []protocol.ChatRequest // recorded requests
}

func (m *mockProvider) Name() string { return "mock" }

func (m *mockProvider) Chat(_ context.Context, req protocol.ChatRequest) (*protocol.ChatResponse, error) {
	m.calls = append(m.calls, req)
	if m.callIdx >= len(m.responses) {
		return nil, fmt.Errorf("mock: no more responses (call %d)", m.callIdx)
	}
	resp := m.responses[m.callIdx]
	m.callIdx++
	return resp, nil
}

// echoTool returns its "text" parameter.
type echoTool struct{}

func (t *echoTool) Name() string        { return "echo" }
func (t *echoTool) Description() string  { return "Echo text" }
func (t *echoTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"text": map[string]any{"type": "string"},
	}}
}
func (t *echoTool) Execute(_ context.Context, params map[string]any) (string, error) {
	v, _ := params["text"].(string)
	return v, nil
}

func TestLoop_DirectResponse(t *testing.T) {
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			{Content: "Hello!"},
		},
	}

	reg := tool.NewRegistry()
	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "You are a test agent."},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	result, err := a.Run(context.Background(), "Hi")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Hello!" {
		t.Errorf("expected 'Hello!', got %q", result)
	}
	if len(prov.calls) != 1 {
		t.Errorf("expected 1 provider call, got %d", len(prov.calls))
	}
	// Verify system + user messages were sent
	msgs := prov.calls[0].Messages
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected system message first, got %q", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("expected user message second, got %q", msgs[1].Role)
	}
}

func TestLoop_ToolCallThenResponse(t *testing.T) {
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			// First call: LLM requests echo tool
			{
				ToolCalls: []protocol.ToolCall{
					{ID: "call_1", Name: "echo", Arguments: map[string]any{"text": "world"}},
				},
			},
			// Second call: LLM returns final text
			{Content: "The echo said: world"},
		},
	}

	reg := tool.NewRegistry()
	reg.Register(&echoTool{})

	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "You are a test agent."},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	result, err := a.Run(context.Background(), "Echo world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "The echo said: world" {
		t.Errorf("expected 'The echo said: world', got %q", result)
	}
	if len(prov.calls) != 2 {
		t.Errorf("expected 2 provider calls, got %d", len(prov.calls))
	}

	// Second call should have: system + user + assistant(tool_calls) + tool result = 4 messages
	msgs := prov.calls[1].Messages
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages in second call, got %d", len(msgs))
	}
	if msgs[2].Role != "assistant" {
		t.Errorf("expected assistant message at index 2, got %q", msgs[2].Role)
	}
	if msgs[3].Role != "tool" {
		t.Errorf("expected tool message at index 3, got %q", msgs[3].Role)
	}
	if msgs[3].Content != "world" {
		t.Errorf("expected tool result 'world', got %q", msgs[3].Content)
	}
	if msgs[3].ToolCallID != "call_1" {
		t.Errorf("expected tool_call_id 'call_1', got %q", msgs[3].ToolCallID)
	}
}

func TestLoop_MultipleToolCalls(t *testing.T) {
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			{
				ToolCalls: []protocol.ToolCall{
					{ID: "c1", Name: "echo", Arguments: map[string]any{"text": "a"}},
					{ID: "c2", Name: "echo", Arguments: map[string]any{"text": "b"}},
				},
			},
			{Content: "done"},
		},
	}

	reg := tool.NewRegistry()
	reg.Register(&echoTool{})

	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "test"},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	result, err := a.Run(context.Background(), "go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got %q", result)
	}

	// Second call: system + user + assistant + tool(c1) + tool(c2) = 5 messages
	msgs := prov.calls[1].Messages
	if len(msgs) != 5 {
		t.Fatalf("expected 5 messages, got %d", len(msgs))
	}
}

func TestLoop_MaxIterations(t *testing.T) {
	// Provider always returns tool calls, never converges
	infToolCall := &protocol.ChatResponse{
		ToolCalls: []protocol.ToolCall{
			{ID: "c", Name: "echo", Arguments: map[string]any{"text": "x"}},
		},
	}
	responses := make([]*protocol.ChatResponse, 5)
	for i := range responses {
		responses[i] = infToolCall
	}

	prov := &mockProvider{responses: responses}
	reg := tool.NewRegistry()
	reg.Register(&echoTool{})

	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "test"},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 3,
	}

	_, err := a.Run(context.Background(), "loop forever")
	if err == nil {
		t.Fatal("expected max iterations error")
	}
	if len(prov.calls) != 3 {
		t.Errorf("expected 3 provider calls, got %d", len(prov.calls))
	}
}

func TestLoop_UnknownTool(t *testing.T) {
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			{
				ToolCalls: []protocol.ToolCall{
					{ID: "c1", Name: "nonexistent", Arguments: nil},
				},
			},
			{Content: "recovered"},
		},
	}

	reg := tool.NewRegistry()
	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "test"},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	result, err := a.Run(context.Background(), "try unknown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "recovered" {
		t.Errorf("expected 'recovered', got %q", result)
	}

	// Tool error should be passed back as tool message
	msgs := prov.calls[1].Messages
	toolMsg := msgs[3]
	if toolMsg.Role != "tool" {
		t.Errorf("expected tool role, got %q", toolMsg.Role)
	}
	if toolMsg.Content == "" {
		t.Error("expected error message in tool content")
	}
}

func TestLoop_ContextCancelled(t *testing.T) {
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{{Content: "should not reach"}},
	}

	reg := tool.NewRegistry()
	a := &Agent{
		Spec:          protocol.AgentSpec{ID: "test", CoreInstructions: "test"},
		Provider:      prov,
		Tools:         reg,
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := a.Run(ctx, "cancelled")
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

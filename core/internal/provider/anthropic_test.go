package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func TestAnthropicChat_TextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Error("missing x-api-key header")
		}
		if r.Header.Get("anthropic-version") != anthropicAPIVersion {
			t.Error("missing anthropic-version header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type")
		}

		var req anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "claude-sonnet-4-20250514" {
			t.Errorf("expected default model, got %s", req.Model)
		}
		if req.MaxTokens != 4096 {
			t.Errorf("expected default max_tokens 4096, got %d", req.MaxTokens)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(req.Messages))
		}

		resp := anthropicResponse{
			Content: []contentBlock{{Type: "text", Text: "Hello!"}},
			Usage:   anthropicUsage{InputTokens: 10, OutputTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	got, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", got.Content)
	}
	if got.HasToolCalls() {
		t.Error("expected no tool calls")
	}
	if got.Usage.PromptTokens != 10 {
		t.Errorf("expected 10 prompt tokens, got %d", got.Usage.PromptTokens)
	}
	if got.Usage.CompletionTokens != 5 {
		t.Errorf("expected 5 completion tokens, got %d", got.Usage.CompletionTokens)
	}
}

func TestAnthropicChat_SystemPrompt(t *testing.T) {
	var capturedReq anthropicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := anthropicResponse{
			Content: []contentBlock{{Type: "text", Text: "OK"}},
			Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{
			{Role: "system", Content: "You are a helpful assistant."},
			{Role: "user", Content: "Hi"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.System != "You are a helpful assistant." {
		t.Errorf("system = %q", capturedReq.System)
	}
	// System message should NOT appear in messages array
	if len(capturedReq.Messages) != 1 {
		t.Fatalf("expected 1 message (system extracted), got %d", len(capturedReq.Messages))
	}
}

func TestAnthropicChat_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req anthropicRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify tools were sent
		if len(req.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(req.Tools))
		}
		if req.Tools[0].Name != "read_file" {
			t.Errorf("expected tool name 'read_file', got %q", req.Tools[0].Name)
		}

		resp := anthropicResponse{
			Content: []contentBlock{
				{Type: "text", Text: "Let me read that file."},
				{
					Type:  "tool_use",
					ID:    "toolu_123",
					Name:  "read_file",
					Input: map[string]any{"path": "/tmp/test.txt"},
				},
			},
			Usage:      anthropicUsage{InputTokens: 20, OutputTokens: 10},
			StopReason: "tool_use",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	got, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Read the file"}},
		Tools: []protocol.ToolDefinition{
			protocol.NewToolDefinition("read_file", "Read a file", map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string"},
				},
				"required": []string{"path"},
			}),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Content != "Let me read that file." {
		t.Errorf("content = %q", got.Content)
	}
	if !got.HasToolCalls() {
		t.Fatal("expected tool calls")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "toolu_123" {
		t.Errorf("tool call ID = %q", tc.ID)
	}
	if tc.Name != "read_file" {
		t.Errorf("tool name = %q", tc.Name)
	}
	if tc.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("path = %v", tc.Arguments["path"])
	}
}

func TestAnthropicChat_ToolResultConversion(t *testing.T) {
	var capturedReq anthropicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := anthropicResponse{
			Content: []contentBlock{{Type: "text", Text: "The file contains hello."}},
			Usage:   anthropicUsage{InputTokens: 30, OutputTokens: 8},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{
			{Role: "user", Content: "Read the file"},
			{
				Role: "assistant",
				ToolCalls: []protocol.ToolCall{{
					ID:        "toolu_123",
					Name:      "read_file",
					Arguments: map[string]any{"path": "/tmp/test.txt"},
				}},
			},
			{Role: "tool", ToolCallID: "toolu_123", Content: "hello world"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify: user message, assistant with tool_use block, user with tool_result block
	if len(capturedReq.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(capturedReq.Messages))
	}

	// Assistant message should have tool_use content block
	assistantMsg := capturedReq.Messages[1]
	if assistantMsg.Role != "assistant" {
		t.Errorf("message[1] role = %q", assistantMsg.Role)
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected 1 content block in assistant msg, got %d", len(assistantMsg.Content))
	}
	if assistantMsg.Content[0].Type != "tool_use" {
		t.Errorf("assistant block type = %q", assistantMsg.Content[0].Type)
	}

	// Tool result should be user role with tool_result content block
	toolMsg := capturedReq.Messages[2]
	if toolMsg.Role != "user" {
		t.Errorf("tool result role = %q (expected 'user')", toolMsg.Role)
	}
	if toolMsg.Content[0].Type != "tool_result" {
		t.Errorf("tool result block type = %q", toolMsg.Content[0].Type)
	}
	if toolMsg.Content[0].ToolUseID != "toolu_123" {
		t.Errorf("tool_use_id = %q", toolMsg.Content[0].ToolUseID)
	}
}

func TestAnthropicChat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"message": "rate limited"}}`))
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
}

func TestAnthropicChat_CustomModel(t *testing.T) {
	var capturedReq anthropicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := anthropicResponse{
			Content: []contentBlock{{Type: "text", Text: "OK"}},
			Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key",
		WithAnthropicBaseURL(srv.URL),
		WithAnthropicModel("claude-haiku-4-5-20251001"),
	)

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %q", capturedReq.Model)
	}
}

func TestAnthropicChat_RequestModelOverride(t *testing.T) {
	var capturedReq anthropicRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedReq)
		resp := anthropicResponse{
			Content: []contentBlock{{Type: "text", Text: "OK"}},
			Usage:   anthropicUsage{InputTokens: 5, OutputTokens: 2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropic("test-key", WithAnthropicBaseURL(srv.URL))

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Model:    "claude-opus-4-20250514",
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedReq.Model != "claude-opus-4-20250514" {
		t.Errorf("model = %q (expected request-level override)", capturedReq.Model)
	}
}

func TestAnthropicProviderName(t *testing.T) {
	p := NewAnthropic("test-key")
	if p.Name() != "anthropic" {
		t.Errorf("Name() = %q", p.Name())
	}
}

func TestToAnthropicMessages_MultipleSystemMessages(t *testing.T) {
	system, msgs := toAnthropicMessages([]protocol.ChatMessage{
		{Role: "system", Content: "First system."},
		{Role: "system", Content: "Second system."},
		{Role: "user", Content: "Hi"},
	})

	if system != "First system.\n\nSecond system." {
		t.Errorf("system = %q", system)
	}
	if len(msgs) != 1 {
		t.Errorf("expected 1 message, got %d", len(msgs))
	}
}

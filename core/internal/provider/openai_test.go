package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func TestOpenAIChat_TextResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing auth header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("missing content-type")
		}

		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "gpt-4o" {
			t.Errorf("expected model gpt-4o, got %s", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Fatalf("expected 1 message, got %d", len(req.Messages))
		}

		resp := openaiResponse{
			Choices: []openaiChoice{{
				Message: openaiMessage{Role: "assistant", Content: "Hello!"},
			}},
			Usage: openaiUsage{PromptTokens: 10, CompletionTokens: 5},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAI("test-key", WithBaseURL(srv.URL))

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
	if got.Usage.TotalTokens() != 15 {
		t.Errorf("expected 15 total tokens, got %d", got.Usage.TotalTokens())
	}
}

func TestOpenAIChat_ToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{{
				Message: openaiMessage{
					Role:    "assistant",
					Content: "",
					ToolCalls: []openaiToolCall{{
						ID:   "call_123",
						Type: "function",
						Function: openaiToolFunction{
							Name:      "read_file",
							Arguments: `{"path": "/tmp/test.txt"}`,
						},
					}},
				},
			}},
			Usage: openaiUsage{PromptTokens: 20, CompletionTokens: 10},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAI("test-key", WithBaseURL(srv.URL))

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
	if !got.HasToolCalls() {
		t.Fatal("expected tool calls")
	}
	if len(got.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got.ToolCalls))
	}
	tc := got.ToolCalls[0]
	if tc.ID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got %q", tc.ID)
	}
	if tc.Name != "read_file" {
		t.Errorf("expected tool name 'read_file', got %q", tc.Name)
	}
	if tc.Arguments["path"] != "/tmp/test.txt" {
		t.Errorf("expected path '/tmp/test.txt', got %v", tc.Arguments["path"])
	}
}

func TestOpenAIChat_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error": {"message": "rate limited"}}`))
	}))
	defer srv.Close()

	p := NewOpenAI("test-key", WithBaseURL(srv.URL))

	_, err := p.Chat(context.Background(), protocol.ChatRequest{
		Messages: []protocol.ChatMessage{{Role: "user", Content: "Hi"}},
	})
	if err == nil {
		t.Fatal("expected error for 429 status")
	}
}

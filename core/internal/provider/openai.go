package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// OpenAIProvider implements Provider for any OpenAI-compatible API
// (OpenAI, OpenRouter, DeepSeek, Groq, etc.).
type OpenAIProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// OpenAIOption configures an OpenAIProvider.
type OpenAIOption func(*OpenAIProvider)

// WithBaseURL sets a custom API base URL.
func WithBaseURL(url string) OpenAIOption {
	return func(p *OpenAIProvider) { p.baseURL = url }
}

// WithModel sets the default model.
func WithModel(model string) OpenAIOption {
	return func(p *OpenAIProvider) { p.model = model }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) OpenAIOption {
	return func(p *OpenAIProvider) { p.client = c }
}

// NewOpenAI creates a new OpenAI-compatible provider.
func NewOpenAI(apiKey string, opts ...OpenAIOption) *OpenAIProvider {
	p := &OpenAIProvider{
		client: &http.Client{Timeout: 120 * time.Second},
		baseURL: "https://api.openai.com/v1",
		apiKey:  apiKey,
		model:   "gpt-4o",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Chat(ctx context.Context, req protocol.ChatRequest) (*protocol.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	body := openaiRequest{
		Model:    model,
		Messages: toOpenAIMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		body.Tools = req.Tools
	}
	if req.MaxTokens > 0 {
		body.MaxTokens = &req.MaxTokens
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var oaiResp openaiResponse
	if err := json.Unmarshal(respBody, &oaiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return parseResponse(&oaiResp)
}

// --- OpenAI wire format types ---

type openaiRequest struct {
	Model       string                 `json:"model"`
	Messages    []openaiMessage        `json:"messages"`
	Tools       []protocol.ToolDefinition `json:"tools,omitempty"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Temperature *float64               `json:"temperature,omitempty"`
}

type openaiMessage struct {
	Role       string              `json:"role"`
	Content    string              `json:"content"`
	ToolCalls  []openaiToolCall     `json:"tool_calls,omitempty"`
	ToolCallID string              `json:"tool_call_id,omitempty"`
	Name       string              `json:"name,omitempty"`
}

type openaiToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openaiToolFunction `json:"function"`
}

type openaiToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// --- Conversion helpers ---

func toOpenAIMessages(msgs []protocol.ChatMessage) []openaiMessage {
	out := make([]openaiMessage, len(msgs))
	for i, m := range msgs {
		om := openaiMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			args, _ := json.Marshal(tc.Arguments)
			om.ToolCalls = append(om.ToolCalls, openaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: openaiToolFunction{
					Name:      tc.Name,
					Arguments: string(args),
				},
			})
		}
		out[i] = om
	}
	return out
}

func parseResponse(resp *openaiResponse) (*protocol.ChatResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	msg := resp.Choices[0].Message

	var toolCalls []protocol.ToolCall
	for _, tc := range msg.ToolCalls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		toolCalls = append(toolCalls, protocol.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: args,
		})
	}

	return &protocol.ChatResponse{
		Content:   msg.Content,
		ToolCalls: toolCalls,
		Usage: protocol.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
		},
	}, nil
}

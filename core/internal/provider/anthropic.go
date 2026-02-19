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

const anthropicAPIVersion = "2023-06-01"

// AnthropicProvider implements Provider for the Anthropic Messages API.
type AnthropicProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// AnthropicOption configures an AnthropicProvider.
type AnthropicOption func(*AnthropicProvider)

// WithAnthropicBaseURL sets a custom API base URL.
func WithAnthropicBaseURL(url string) AnthropicOption {
	return func(p *AnthropicProvider) { p.baseURL = url }
}

// WithAnthropicModel sets the default model.
func WithAnthropicModel(model string) AnthropicOption {
	return func(p *AnthropicProvider) { p.model = model }
}

// NewAnthropic creates a new Anthropic Messages API provider.
func NewAnthropic(apiKey string, opts ...AnthropicOption) *AnthropicProvider {
	p := &AnthropicProvider{
		client:  &http.Client{Timeout: 120 * time.Second},
		baseURL: "https://api.anthropic.com",
		apiKey:  apiKey,
		model:   "claude-sonnet-4-20250514",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Chat(ctx context.Context, req protocol.ChatRequest) (*protocol.ChatResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}

	// Convert protocol messages to Anthropic format
	system, messages := toAnthropicMessages(req.Messages)

	body := anthropicRequest{
		Model:    model,
		Messages: messages,
		System:   system,
	}

	if req.MaxTokens > 0 {
		body.MaxTokens = req.MaxTokens
	} else {
		body.MaxTokens = 4096 // Anthropic requires max_tokens
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}

	// Convert tools to Anthropic format
	if len(req.Tools) > 0 {
		for _, td := range req.Tools {
			body.Tools = append(body.Tools, anthropicTool{
				Name:        td.Function.Name,
				Description: td.Function.Description,
				InputSchema: td.Function.Parameters,
			})
		}
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var anthResp anthropicResponse
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	return parseAnthropicResponse(&anthResp)
}

// --- Anthropic wire format types ---

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Tools       []anthropicTool    `json:"tools,omitempty"`
}

type anthropicMessage struct {
	Role    string        `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock is a union type for Anthropic content blocks.
// Uses a custom marshaler to emit only fields relevant to each block type.
type contentBlock struct {
	Type      string         `json:"-"`
	Text      string         `json:"-"`
	ID        string         `json:"-"`
	Name      string         `json:"-"`
	Input     map[string]any `json:"-"`
	ToolUseID string         `json:"-"`
	Content   string         `json:"-"` // used for tool_result content
}

func (b contentBlock) MarshalJSON() ([]byte, error) {
	switch b.Type {
	case "tool_use":
		input := b.Input
		if input == nil {
			input = map[string]any{}
		}
		return json.Marshal(struct {
			Type  string         `json:"type"`
			ID    string         `json:"id"`
			Name  string         `json:"name"`
			Input map[string]any `json:"input"`
		}{b.Type, b.ID, b.Name, input})
	case "tool_result":
		return json.Marshal(struct {
			Type      string `json:"type"`
			ToolUseID string `json:"tool_use_id"`
			Content   string `json:"content"`
		}{b.Type, b.ToolUseID, b.Content})
	default: // "text"
		return json.Marshal(struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}{b.Type, b.Text})
	}
}

func (b *contentBlock) UnmarshalJSON(data []byte) error {
	var raw struct {
		Type      string         `json:"type"`
		Text      string         `json:"text"`
		ID        string         `json:"id"`
		Name      string         `json:"name"`
		Input     map[string]any `json:"input"`
		ToolUseID string         `json:"tool_use_id"`
		Content   string         `json:"content"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	b.Type = raw.Type
	b.Text = raw.Text
	b.ID = raw.ID
	b.Name = raw.Name
	b.Input = raw.Input
	b.ToolUseID = raw.ToolUseID
	b.Content = raw.Content
	return nil
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
	Usage   anthropicUsage `json:"usage"`
	StopReason string      `json:"stop_reason"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- Conversion helpers ---

// toAnthropicMessages converts protocol messages to Anthropic format.
// Extracts system messages as a separate top-level field.
func toAnthropicMessages(msgs []protocol.ChatMessage) (string, []anthropicMessage) {
	var system string
	var result []anthropicMessage

	for _, m := range msgs {
		if m.Role == "system" {
			if system != "" {
				system += "\n\n"
			}
			system += m.Content
			continue
		}

		if m.Role == "tool" {
			// Tool results in Anthropic format
			result = append(result, anthropicMessage{
				Role: "user",
				Content: []contentBlock{{
					Type:      "tool_result",
					ToolUseID: m.ToolCallID,
					Content:   m.Content,
				}},
			})
			continue
		}

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Assistant with tool calls
			var blocks []contentBlock
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				input := tc.Arguments
				if input == nil {
					input = map[string]any{}
				}
				blocks = append(blocks, contentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: input,
				})
			}
			result = append(result, anthropicMessage{Role: "assistant", Content: blocks})
			continue
		}

		// Regular text message
		result = append(result, anthropicMessage{
			Role:    m.Role,
			Content: []contentBlock{{Type: "text", Text: m.Content}},
		})
	}

	return system, result
}

func parseAnthropicResponse(resp *anthropicResponse) (*protocol.ChatResponse, error) {
	var content string
	var toolCalls []protocol.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			toolCalls = append(toolCalls, protocol.ToolCall{
				ID:        block.ID,
				Name:      block.Name,
				Arguments: block.Input,
			})
		}
	}

	return &protocol.ChatResponse{
		Content:   content,
		ToolCalls: toolCalls,
		Usage: protocol.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
		},
	}, nil
}

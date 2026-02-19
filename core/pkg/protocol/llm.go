package protocol

// ChatMessage represents a single message in the LLM conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall represents the LLM requesting a tool execution.
type ToolCall struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// ChatResponse is the parsed response from an LLM provider.
type ChatResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// HasToolCalls returns true if the response contains tool call requests.
func (r *ChatResponse) HasToolCalls() bool {
	return len(r.ToolCalls) > 0
}

// Usage tracks token consumption for a single LLM call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// TotalTokens returns the sum of prompt and completion tokens.
func (u Usage) TotalTokens() int {
	return u.PromptTokens + u.CompletionTokens
}

// ChatRequest holds parameters for an LLM chat call.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []ChatMessage    `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

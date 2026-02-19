package protocol

// ToolDefinition describes a tool available to the LLM (OpenAI function-calling format).
type ToolDefinition struct {
	Type     string             `json:"type"`
	Function ToolFunctionSchema `json:"function"`
}

// ToolFunctionSchema is the function schema within a tool definition.
type ToolFunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// NewToolDefinition creates a ToolDefinition in OpenAI function-calling format.
func NewToolDefinition(name, description string, parameters map[string]any) ToolDefinition {
	return ToolDefinition{
		Type: "function",
		Function: ToolFunctionSchema{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

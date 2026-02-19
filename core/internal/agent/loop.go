package agent

import (
	"context"
	"fmt"

	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Run executes the ReAct loop: send messages to the LLM, execute any requested
// tool calls, and loop until the LLM returns a final text response or the
// iteration limit is reached.
func (a *Agent) Run(ctx context.Context, userMessage string) (string, error) {
	messages := []protocol.ChatMessage{
		{Role: "system", Content: a.Spec.CoreInstructions},
		{Role: "user", Content: userMessage},
	}
	return a.runLoop(ctx, messages)
}

// RunWithHistory executes the ReAct loop with an existing conversation history.
func (a *Agent) RunWithHistory(ctx context.Context, messages []protocol.ChatMessage) (string, error) {
	return a.runLoop(ctx, messages)
}

func (a *Agent) runLoop(ctx context.Context, messages []protocol.ChatMessage) (string, error) {
	maxIter := a.MaxIterations
	if maxIter <= 0 {
		maxIter = defaultMaxIterations
	}

	toolDefs := a.Tools.Definitions()

	for i := 0; i < maxIter; i++ {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("agent %s: context cancelled: %w", a.Spec.ID, err)
		}

		req := protocol.ChatRequest{
			Messages: messages,
			Tools:    toolDefs,
		}

		a.Logger.Debug("agent chat request",
			"agent", a.Spec.ID,
			"iteration", i+1,
			"messages", len(messages),
		)

		resp, err := a.Provider.Chat(ctx, req)
		if err != nil {
			return "", fmt.Errorf("agent %s: provider error: %w", a.Spec.ID, err)
		}

		if !resp.HasToolCalls() {
			a.Logger.Debug("agent final response",
				"agent", a.Spec.ID,
				"iteration", i+1,
				"content_len", len(resp.Content),
			)
			return resp.Content, nil
		}

		// Append assistant message with tool calls
		messages = append(messages, protocol.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Execute each tool call and append results
		ticketID := tool.CurrentTicketFromContext(ctx)
		for _, tc := range resp.ToolCalls {
			a.Logger.Info(fmt.Sprintf("tool call: %s", tc.Name),
				"agent", a.Spec.ID,
				"ticket", ticketID,
				"call_id", tc.ID,
			)

			result, err := a.Tools.Execute(ctx, tc.Name, tc.Arguments)
			if err != nil {
				// Return error as tool result so the LLM can recover
				result = fmt.Sprintf("Error: %v", err)
				a.Logger.Warn(fmt.Sprintf("tool error: %s", tc.Name),
					"agent", a.Spec.ID,
					"ticket", ticketID,
					"error", err,
				)
			} else {
				a.Logger.Info(fmt.Sprintf("tool result: %s", tc.Name),
					"agent", a.Spec.ID,
					"ticket", ticketID,
					"result_len", len(result),
				)
			}

			messages = append(messages, protocol.ChatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: tc.ID,
				Name:       tc.Name,
			})
		}

		// If the agent already sent a response via respond_to_ticket,
		// exit immediately — no need for another LLM round-trip.
		if tool.Responded(ctx) {
			return "", nil
		}
	}

	return "", fmt.Errorf("agent %s: exceeded max iterations (%d)", a.Spec.ID, maxIter)
}

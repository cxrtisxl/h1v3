package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Consolidator extracts learnings from closed tickets into agent memory.
type Consolidator struct {
	Provider provider.Provider
	Model    string
}

// Consolidate analyzes a closed ticket and merges extracted learnings into the agent's memory store.
// The LLM returns JSON {scope: content_to_append} and each scope is appended to existing content.
func (c *Consolidator) Consolidate(ctx context.Context, ticket *protocol.Ticket, store *Store) error {
	if len(ticket.Messages) == 0 {
		return nil
	}

	// Build conversation context
	var conv strings.Builder
	fmt.Fprintf(&conv, "Ticket: %s\n", ticket.Title)
	fmt.Fprintf(&conv, "Status: %s\n\n", ticket.Status)
	for _, m := range ticket.Messages {
		fmt.Fprintf(&conv, "[%s]: %s\n", m.From, m.Content)
	}

	// Include existing memory scopes for context
	existing := store.List()
	var memCtx strings.Builder
	if len(existing) > 0 {
		memCtx.WriteString("Existing memory scopes:\n")
		for scope, content := range existing {
			fmt.Fprintf(&memCtx, "--- %s ---\n%s\n", scope, content)
		}
	}

	req := protocol.ChatRequest{
		Model: c.Model,
		Messages: []protocol.ChatMessage{
			{
				Role: "system",
				Content: `You are a memory consolidation assistant. Analyze the completed ticket conversation and extract important facts, decisions, or preferences that should be remembered long-term.

Return ONLY a JSON object where keys are memory scope names and values are the content to append to that scope. Use these standard scopes when appropriate:
- "project" — project-specific knowledge, architecture decisions, patterns
- "preferences" — user or team preferences
- "team" — team structure, roles, contact info

You may also create custom scopes if needed.

If there is nothing worth remembering, return an empty JSON object: {}

Do not include trivial or transient information. Focus on reusable knowledge.`,
			},
			{
				Role:    "user",
				Content: conv.String() + "\n\n" + memCtx.String(),
			},
		},
		MaxTokens:   512,
		Temperature: 0.2,
	}

	resp, err := c.Provider.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("consolidate: LLM call failed: %w", err)
	}

	// Parse JSON response
	learnings, err := parseConsolidationResponse(resp.Content)
	if err != nil {
		return fmt.Errorf("consolidate: parse response: %w", err)
	}

	// Merge learnings into memory store
	for scope, newContent := range learnings {
		if newContent == "" {
			continue
		}
		existing := store.Get(scope)
		if existing != "" {
			newContent = existing + "\n\n" + newContent
		}
		if err := store.Set(scope, newContent); err != nil {
			return fmt.Errorf("consolidate: update scope %q: %w", scope, err)
		}
	}

	return nil
}

// parseConsolidationResponse extracts a JSON object from the LLM response.
// Handles cases where the JSON is wrapped in markdown code fences.
func parseConsolidationResponse(content string) (map[string]string, error) {
	content = strings.TrimSpace(content)

	// Strip markdown code fences if present
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		if len(lines) >= 3 {
			// Remove first and last lines (fences)
			content = strings.Join(lines[1:len(lines)-1], "\n")
			content = strings.TrimSpace(content)
		}
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("invalid JSON from LLM: %w (content: %q)", err, content)
	}
	return result, nil
}

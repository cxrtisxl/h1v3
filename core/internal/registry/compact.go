package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/h1v3-io/h1v3/internal/provider"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Compactor summarizes old ticket messages to reduce token usage.
type Compactor struct {
	Provider  provider.Provider
	Model     string
	Threshold int // estimated token count that triggers compaction
	Keep      int // number of recent messages to preserve (default 4)
}

// EstimateTokens returns a rough token estimate for a string (words × 1.3).
func EstimateTokens(s string) int {
	words := len(strings.Fields(s))
	return int(float64(words) * 1.3)
}

// ticketTokens estimates the total tokens in a ticket's messages.
func ticketTokens(t *protocol.Ticket) int {
	total := 0
	for _, m := range t.Messages {
		total += EstimateTokens(m.Content)
	}
	return total
}

// ShouldCompact returns true if the ticket's estimated token count exceeds the threshold.
func (c *Compactor) ShouldCompact(ticket *protocol.Ticket) bool {
	if c.Threshold <= 0 {
		return false
	}
	return ticketTokens(ticket) > c.Threshold
}

// Compact summarizes old messages in-place, keeping the most recent ones intact.
// It replaces the oldest messages with a single summary message.
func (c *Compactor) Compact(ctx context.Context, ticket *protocol.Ticket) error {
	keep := c.Keep
	if keep <= 0 {
		keep = 4
	}

	msgs := ticket.Messages
	if len(msgs) <= keep+1 {
		return nil // not enough messages to compact
	}

	// Split: old messages to summarize, recent to keep
	oldMsgs := msgs[:len(msgs)-keep]
	recentMsgs := msgs[len(msgs)-keep:]

	// Build conversation text for summarization
	var conv strings.Builder
	for _, m := range oldMsgs {
		fmt.Fprintf(&conv, "[%s → %s]: %s\n", m.From, strings.Join(m.To, ","), m.Content)
	}

	// Ask the LLM to summarize
	req := protocol.ChatRequest{
		Model: c.Model,
		Messages: []protocol.ChatMessage{
			{
				Role:    "system",
				Content: "Summarize the following conversation, preserving key decisions, action items, and important context. Be concise but thorough.",
			},
			{
				Role:    "user",
				Content: conv.String(),
			},
		},
		MaxTokens:   512,
		Temperature: 0.2,
	}

	resp, err := c.Provider.Chat(ctx, req)
	if err != nil {
		return fmt.Errorf("compact: LLM summarization failed: %w", err)
	}

	// Build compacted message list
	summary := protocol.Message{
		From:    "system",
		Content: fmt.Sprintf("[Compacted: %s]", resp.Content),
	}

	ticket.Messages = append([]protocol.Message{summary}, recentMsgs...)
	return nil
}

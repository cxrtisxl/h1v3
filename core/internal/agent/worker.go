package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

const (
	maxRetries   = 3
	retryDelay   = 10 * time.Second
)

// MessageRouter routes messages between agents. This interface breaks the
// import cycle between agent and registry packages.
type MessageRouter interface {
	RouteMessage(msg protocol.Message) error
	GetTicket(ticketID string) (*protocol.Ticket, error)
}

// Worker runs an agent's event loop, processing messages from an inbox channel.
type Worker struct {
	Agent  *Agent
	Inbox  <-chan protocol.Message
	Router MessageRouter
}

// Start runs the agent's message processing loop. It blocks until the context
// is cancelled or the inbox channel is closed.
func (w *Worker) Start(ctx context.Context) error {
	w.Agent.Logger.Info("agent worker started", "agent", w.Agent.Spec.ID)

	for {
		select {
		case msg, ok := <-w.Inbox:
			if !ok {
				w.Agent.Logger.Info("agent inbox closed", "agent", w.Agent.Spec.ID)
				return nil
			}
			w.handleMessage(ctx, msg, 0)

		case <-ctx.Done():
			w.Agent.Logger.Info("agent worker stopping", "agent", w.Agent.Spec.ID)
			return ctx.Err()
		}
	}
}

func (w *Worker) handleMessage(ctx context.Context, msg protocol.Message, attempt int) {
	agentID := w.Agent.Spec.ID
	w.Agent.Logger.Debug("processing message",
		"agent", agentID,
		"ticket", msg.TicketID,
		"from", msg.From,
	)

	// Load ticket context
	ticket, err := w.Router.GetTicket(msg.TicketID)
	if err != nil {
		w.Agent.Logger.Error("failed to load ticket",
			"agent", agentID,
			"ticket", msg.TicketID,
			"error", err,
		)
		return
	}

	// Build system prompt with ticket context
	systemPrompt := w.Agent.BuildSystemPrompt(ticket)

	// Build conversation: system prompt + ticket messages as context + incoming message
	messages := []protocol.ChatMessage{
		{Role: "system", Content: systemPrompt},
	}

	// Include ticket messages as conversation context.
	// The incoming message is already persisted by RouteMessage, so it's in ticket.Messages.
	for _, m := range ticket.Messages {
		role := "user"
		if m.From == agentID {
			role = "assistant"
		}
		messages = append(messages, protocol.ChatMessage{
			Role:    role,
			Content: fmt.Sprintf("[%s]: %s", m.From, m.Content),
		})
	}

	// Run the ReAct loop with current ticket ID and input messages in context
	ticketCtx := tool.WithCurrentTicket(ctx, msg.TicketID)
	ticketCtx = tool.WithInputMessages(ticketCtx, messages)
	ticketCtx, responded := tool.WithRespondedFlag(ticketCtx)
	response, err := w.Agent.RunWithHistory(ticketCtx, messages)
	if err != nil {
		errContextID := fmt.Sprintf("err-%d", time.Now().UnixNano())

		// Log prompt context for the failed call, with error appended
		errorCtx := append(messages, protocol.ChatMessage{
			Role:    "error",
			Content: err.Error(),
		})
		if ctxJSON, err := json.Marshal(errorCtx); err == nil {
			w.Agent.Logger.Info("prompt_context",
				"msg_id", errContextID,
				"agent", agentID,
				"ticket", msg.TicketID,
				"context", string(ctxJSON),
			)
		}

		w.Agent.Logger.Error("agent LLM error, response blocked",
			"agent", agentID,
			"ticket", msg.TicketID,
			"attempt", attempt+1,
			"error", err,
			"prompt_context_id", errContextID,
		)

		// Retry with delay (up to maxRetries)
		if attempt < maxRetries {
			w.Agent.Logger.Info("scheduling retry",
				"agent", agentID,
				"ticket", msg.TicketID,
				"attempt", attempt+1,
				"delay", retryDelay,
			)
			go func() {
				select {
				case <-time.After(retryDelay):
					w.handleMessage(ctx, msg, attempt+1)
				case <-ctx.Done():
				}
			}()
		} else {
			w.Agent.Logger.Error("max retries exhausted, giving up",
				"agent", agentID,
				"ticket", msg.TicketID,
				"attempts", attempt+1,
			)
		}
		return
	}

	// Skip auto-response if agent already responded via respond_to_ticket,
	// or if the final LLM output is empty.
	if *responded || strings.TrimSpace(response) == "" {
		return
	}

	// Route response to all ticket participants (except self)
	recipients := ticketRecipients(ticket, agentID)
	respMsg := protocol.Message{
		ID:        fmt.Sprintf("resp-%d", time.Now().UnixNano()),
		From:      agentID,
		To:        recipients,
		Content:   response,
		TicketID:  msg.TicketID,
		Timestamp: time.Now(),
	}

	// Log prompt context for this auto-response
	if ctxJSON, err := json.Marshal(messages); err == nil {
		w.Agent.Logger.Info("prompt_context",
			"msg_id", respMsg.ID,
			"agent", agentID,
			"ticket", msg.TicketID,
			"context", string(ctxJSON),
		)
	}

	if err := w.Router.RouteMessage(respMsg); err != nil {
		w.Agent.Logger.Error("failed to route response",
			"agent", agentID,
			"ticket", msg.TicketID,
			"error", err,
		)
	}
}

// ticketRecipients returns all ticket participants except the sender.
func ticketRecipients(tk *protocol.Ticket, sender string) []string {
	seen := map[string]bool{sender: true}
	var out []string
	for _, id := range append([]string{tk.CreatedBy}, tk.WaitingOn...) {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}

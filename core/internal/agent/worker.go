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
	ListSubTickets(parentID string) ([]*protocol.Ticket, error)
	UpdateTicketStatus(ticketID string, status protocol.TicketStatus) error
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

	// Load sub-tickets for context
	var subTickets []*protocol.Ticket
	if subs, err := w.Router.ListSubTickets(ticket.ID); err == nil {
		subTickets = subs
	}

	// Build system prompt with ticket context
	systemPrompt := w.Agent.BuildSystemPrompt(ticket, subTickets)

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
	ticketCtx, deferredMsgs := tool.WithDeferredMessages(ticketCtx)
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

	// If the agent returned plain text without calling respond_to_ticket,
	// nudge it to use the tool and re-run once.
	if !*responded && strings.TrimSpace(response) != "" {
		w.Agent.Logger.Warn("agent returned plain text without calling respond_to_ticket, retrying with nudge",
			"agent", agentID,
			"ticket", msg.TicketID,
		)
		nudgeMessages := append(messages,
			protocol.ChatMessage{Role: "assistant", Content: response},
			protocol.ChatMessage{
				Role:    "user",
				Content: "[system] Do not reply with plain text. Use the respond_to_ticket tool to send your response. Set goal_met=true if the goal is satisfied.",
			},
		)
		_, err = w.Agent.RunWithHistory(ticketCtx, nudgeMessages)
		if err != nil {
			w.Agent.Logger.Error("nudge retry failed",
				"agent", agentID,
				"ticket", msg.TicketID,
				"error", err,
			)
		}
	}

	// Flush deferred messages (respond_to_ticket on the current ticket).
	// RouteMessage checks ticket status and skips inbox delivery on closed tickets.
	for _, dm := range *deferredMsgs {
		if err := w.Router.RouteMessage(dm); err != nil {
			w.Agent.Logger.Error("failed to deliver deferred message",
				"agent", agentID,
				"ticket", dm.TicketID,
				"error", err,
			)
		}
	}
}

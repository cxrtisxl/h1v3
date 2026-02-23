package tool

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// TicketBroker abstracts ticket operations. Implemented by the registry
// adapter in cmd/h1v3d to break the import cycle.
type TicketBroker interface {
	CreateTicket(from, title, goal, parentID string, to, tags []string) (*protocol.Ticket, error)
	GetTicket(ticketID string) (*protocol.Ticket, error)
	ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error)
	CountTickets(filter ticket.Filter) (int, error)
	CloseTicket(ticketID, summary string) error
	UpdateTicketStatus(ticketID string, status protocol.TicketStatus) error
	RouteMessage(msg protocol.Message) error
}

// contextKey is an unexported type for context keys in this package.
type contextKey string

// TicketContextKey is the context key for the current ticket ID.
const TicketContextKey = contextKey("current_ticket_id")

// inputMessagesKey is the context key for the input messages slice.
const inputMessagesKey = contextKey("input_messages")

// respondedKey is the context key for the responded flag (*bool).
const respondedKey = contextKey("responded")

// deferredMsgsKey is the context key for deferred message delivery.
const deferredMsgsKey = contextKey("deferred_messages")

// WithCurrentTicket returns a context with the current ticket ID set.
func WithCurrentTicket(ctx context.Context, ticketID string) context.Context {
	return context.WithValue(ctx, TicketContextKey, ticketID)
}

// CurrentTicketFromContext returns the ticket ID from the context, if any.
func CurrentTicketFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(TicketContextKey).(string); ok {
		return v
	}
	return ""
}

// WithInputMessages returns a context carrying the LLM input messages.
func WithInputMessages(ctx context.Context, msgs []protocol.ChatMessage) context.Context {
	return context.WithValue(ctx, inputMessagesKey, msgs)
}

// InputMessagesFromContext retrieves the LLM input messages from context.
func InputMessagesFromContext(ctx context.Context) []protocol.ChatMessage {
	if v, ok := ctx.Value(inputMessagesKey).([]protocol.ChatMessage); ok {
		return v
	}
	return nil
}

// WithRespondedFlag returns a context carrying a mutable responded flag.
// The flag is set to true when respond_to_ticket is called.
func WithRespondedFlag(ctx context.Context) (context.Context, *bool) {
	flag := new(bool)
	return context.WithValue(ctx, respondedKey, flag), flag
}

// Responded returns true if respond_to_ticket was called in this context.
func Responded(ctx context.Context) bool {
	if flag, ok := ctx.Value(respondedKey).(*bool); ok {
		return *flag
	}
	return false
}

func markResponded(ctx context.Context) {
	if flag, ok := ctx.Value(respondedKey).(*bool); ok {
		*flag = true
	}
}

// WithDeferredMessages returns a context that buffers messages for deferred
// delivery. Messages on the current ticket are deferred so that a subsequent
// close_ticket in the same turn can suppress inbox delivery.
func WithDeferredMessages(ctx context.Context) (context.Context, *[]protocol.Message) {
	msgs := &[]protocol.Message{}
	return context.WithValue(ctx, deferredMsgsKey, msgs), msgs
}

func deferMessage(ctx context.Context, msg protocol.Message) {
	if msgs, ok := ctx.Value(deferredMsgsKey).(*[]protocol.Message); ok {
		*msgs = append(*msgs, msg)
	}
}

// --- helpers ---

func getStringSlice(params map[string]any, key string) []string {
	raw, ok := params[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []any:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}
	return nil
}

func generateMsgID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return fmt.Sprintf("m-%x", b)
}

// collectRecipients returns all ticket participants except the sender.
func collectRecipients(tk *protocol.Ticket, sender string) []string {
	seen := make(map[string]bool)
	seen[sender] = true

	var recipients []string
	if !seen[tk.CreatedBy] {
		recipients = append(recipients, tk.CreatedBy)
		seen[tk.CreatedBy] = true
	}
	for _, id := range tk.WaitingOn {
		if !seen[id] {
			recipients = append(recipients, id)
			seen[id] = true
		}
	}
	return recipients
}

// sameRecipientOverlap returns target agent IDs that are already participants
// (creator or assignee) on the given parent ticket.
func sameRecipientOverlap(parent *protocol.Ticket, to []string) []string {
	participants := make(map[string]bool)
	participants[parent.CreatedBy] = true
	for _, id := range parent.WaitingOn {
		participants[id] = true
	}
	var overlap []string
	for _, id := range to {
		if participants[id] {
			overlap = append(overlap, id)
		}
	}
	return overlap
}

// validateAgentIDs checks that all given IDs are known agents.
// Returns an error listing unknown IDs and the valid ones.
func validateAgentIDs(lister AgentLister, ids []string) error {
	known := make(map[string]bool)
	for _, a := range lister.ListAgentInfo() {
		known[a.ID] = true
	}
	var bad []string
	for _, id := range ids {
		if !known[id] {
			bad = append(bad, id)
		}
	}
	if len(bad) > 0 {
		var valid []string
		for id := range known {
			valid = append(valid, id)
		}
		return fmt.Errorf("unknown agent(s): %s (valid agents: %s)", strings.Join(bad, ", "), strings.Join(valid, ", "))
	}
	return nil
}

// --- CreateTicketTool ---

type CreateTicketTool struct {
	Broker  TicketBroker
	AgentID string
	Agents  AgentLister
}

func (t *CreateTicketTool) Name() string        { return "create_ticket" }
func (t *CreateTicketTool) Description() string  { return "Create a ticket to delegate work to other agents" }
func (t *CreateTicketTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Target agent IDs"},
			"title": map[string]any{"type": "string", "description": "Ticket title describing the task"},
			"goal":  map[string]any{"type": "string", "description": "Concrete completion condition — what response or outcome would satisfy this ticket (e.g. 'Get the agent's display name')"},
			"message":   map[string]any{"type": "string", "description": "Optional free-form message to include with the ticket (e.g. research results, context, supporting data)"},
			"tags":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional tags"},
			"confirmed": map[string]any{"type": "boolean", "description": "Set to true to confirm creating a sub-ticket to the same agent as the parent ticket"},
			"reason":    map[string]any{"type": "string", "description": "Required when confirmed=true — explain why a new sub-ticket is needed instead of using respond_to_ticket, close_ticket, or wait"},
		},
		"required": []string{"to", "title", "goal"},
	}
}

func (t *CreateTicketTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	title := getString(params, "title")
	goal := getString(params, "goal")
	message := getString(params, "message")
	to := getStringSlice(params, "to")
	tags := getStringSlice(params, "tags")

	if title == "" {
		return "", fmt.Errorf("create_ticket: title is required")
	}
	if goal == "" {
		return "", fmt.Errorf("create_ticket: goal is required")
	}
	if len(to) == 0 {
		return "", fmt.Errorf("create_ticket: at least one target agent is required")
	}
	for _, id := range to {
		if id == t.AgentID {
			return "", fmt.Errorf("create_ticket: cannot assign a ticket to yourself — do the work directly")
		}
	}
	if t.Agents != nil {
		if err := validateAgentIDs(t.Agents, to); err != nil {
			return "", fmt.Errorf("create_ticket: %w", err)
		}
	}

	// Auto-set parent ticket from context (the ticket the agent is currently working on)
	parentID := CurrentTicketFromContext(ctx)

	// When creating a sub-ticket, check if any target agent is already a
	// participant on the parent ticket. If so, require explicit confirmation
	// to avoid agents falling into loops of creating sub-tickets to each other.
	if parentID != "" {
		confirmed, _ := params["confirmed"].(bool)
		reason := getString(params, "reason")
		if confirmed && reason == "" {
			return "", fmt.Errorf("create_ticket: reason is required when confirmed=true — explain why a new sub-ticket is needed")
		}
		if !confirmed {
			parentTicket, err := t.Broker.GetTicket(parentID)
			if err == nil {
				// Block sub-ticket creation when parent is awaiting_close
				if parentTicket.Status == protocol.TicketAwaitingClose {
					return "CONFIRMATION REQUIRED: The parent ticket is awaiting_close — the responder believes the goal is met. " +
						"Evaluate the response and close the ticket first. If you genuinely need a new sub-ticket, " +
						"call create_ticket again with confirmed=true AND reason explaining why.", nil
				}
				overlap := sameRecipientOverlap(parentTicket, to)
				if len(overlap) > 0 {
					return fmt.Sprintf(
						"CONFIRMATION REQUIRED: You are creating a sub-ticket for an existing ticket for %s with title %q and goal %q. "+
							"Are you sure the sub-ticket to the same agent with title %q and goal %q should be created? "+
							"Consider these alternatives first:\n"+
							"- `respond_to_ticket` — add more context or ask follow-up questions on the existing ticket\n"+
							"- `close_ticket` — if the assignee has already provided the answer or indicated the work is done\n"+
							"- `wait` — if you are waiting for the assignee to finish\n\n"+
							"To proceed, call create_ticket again with confirmed=true AND reason explaining why a new sub-ticket is necessary.",
						strings.Join(overlap, ", "), parentTicket.Title, parentTicket.Goal, title, goal,
					), nil
				}
			}
		}
	}

	tk, err := t.Broker.CreateTicket(t.AgentID, title, goal, parentID, to, tags)
	if err != nil {
		return "", fmt.Errorf("create_ticket: %w", err)
	}

	// Deliver initial message to target agents via normal routing.
	// Include the goal and optional message in the body so assignees have the full context.
	content := title
	if goal != "" {
		content = title + "\n\n" + goal
	}
	if message != "" {
		content = content + "\n\n" + message
	}
	msg := protocol.Message{
		ID:        generateMsgID(),
		From:      t.AgentID,
		To:        to,
		Content:   content,
		TicketID:  tk.ID,
		Timestamp: time.Now(),
	}
	if err := t.Broker.RouteMessage(msg); err != nil {
		return "", fmt.Errorf("create_ticket: route: %w", err)
	}

	return fmt.Sprintf("Ticket created: %s (title: %q, assigned to: %s)", tk.ID, title, strings.Join(to, ", ")), nil
}

// --- RespondToTicketTool ---

type RespondToTicketTool struct {
	Broker  TicketBroker
	AgentID string
	Logger  *slog.Logger
}

func (t *RespondToTicketTool) Name() string        { return "respond_to_ticket" }
func (t *RespondToTicketTool) Description() string  { return "Send a response on the current ticket" }
func (t *RespondToTicketTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"message":  map[string]any{"type": "string", "description": "Response message"},
			"goal_met": map[string]any{"type": "boolean", "description": "Set to true when your response fully satisfies the ticket's goal (responders only)"},
		},
		"required": []string{"message"},
	}
}

func (t *RespondToTicketTool) Execute(ctx context.Context, params map[string]any) (string, error) {
	ticketID := CurrentTicketFromContext(ctx)
	message := getString(params, "message")

	if ticketID == "" {
		return "", fmt.Errorf("respond_to_ticket: must be called within a ticket context")
	}
	if message == "" {
		return "", fmt.Errorf("respond_to_ticket: message is required")
	}

	tk, err := t.Broker.GetTicket(ticketID)
	if err != nil {
		return "", fmt.Errorf("respond_to_ticket: ticket %q not found", ticketID)
	}

	if tk.Status == protocol.TicketClosed {
		return "Ticket is closed — message not delivered.", nil
	}

	// goal_met validation: only responders (non-creators) may set it
	goalMet, _ := params["goal_met"].(bool)
	if goalMet && tk.CreatedBy == t.AgentID {
		return "", fmt.Errorf("respond_to_ticket: only responders can set goal_met (you are the creator)")
	}

	recipients := collectRecipients(tk, t.AgentID)

	msg := protocol.Message{
		ID:        generateMsgID(),
		From:      t.AgentID,
		To:        recipients,
		Content:   message,
		TicketID:  ticketID,
		Timestamp: time.Now(),
	}

	// Defer delivery so that a close_ticket call later in the same turn
	// can suppress inbox delivery.
	deferMessage(ctx, msg)

	markResponded(ctx)

	// Status transitions
	var statusNote string
	if goalMet && tk.Status == protocol.TicketOpen {
		if err := t.Broker.UpdateTicketStatus(ticketID, protocol.TicketAwaitingClose); err != nil {
			return "", fmt.Errorf("respond_to_ticket: update status: %w", err)
		}
		statusNote = " (status → awaiting_close)"
	} else if tk.Status == protocol.TicketAwaitingClose && tk.CreatedBy == t.AgentID {
		// Creator responding on an awaiting_close ticket reopens it
		if err := t.Broker.UpdateTicketStatus(ticketID, protocol.TicketOpen); err != nil {
			return "", fmt.Errorf("respond_to_ticket: update status: %w", err)
		}
		statusNote = " (status → open)"
	}

	// Log prompt context for this message
	if t.Logger != nil {
		if inputMsgs := InputMessagesFromContext(ctx); inputMsgs != nil {
			if ctxJSON, err := json.Marshal(inputMsgs); err == nil {
				t.Logger.Info("prompt_context",
					"msg_id", msg.ID,
					"agent", t.AgentID,
					"ticket", ticketID,
					"context", string(ctxJSON),
				)
			}
		}
	}

	return fmt.Sprintf("Message sent on ticket %s to %s%s", ticketID, strings.Join(recipients, ", "), statusNote), nil
}

// --- CloseTicketTool ---

type CloseTicketTool struct {
	Broker  TicketBroker
	AgentID string
}

func (t *CloseTicketTool) Name() string        { return "close_ticket" }
func (t *CloseTicketTool) Description() string  { return "Close a ticket with a summary" }
func (t *CloseTicketTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID to close"},
			"summary":   map[string]any{"type": "string", "description": "Summary of what was accomplished"},
		},
		"required": []string{"ticket_id", "summary"},
	}
}

func (t *CloseTicketTool) Execute(_ context.Context, params map[string]any) (string, error) {
	ticketID := getString(params, "ticket_id")
	summary := getString(params, "summary")

	if ticketID == "" || summary == "" {
		return "", fmt.Errorf("close_ticket: ticket_id and summary are required")
	}

	// Only the ticket creator can close it
	tk, err := t.Broker.GetTicket(ticketID)
	if err != nil {
		return "", fmt.Errorf("close_ticket: %w", err)
	}
	if tk.CreatedBy != t.AgentID {
		return fmt.Sprintf("You cannot close this ticket — only the creator (%s) can close it. Use respond_to_ticket to send your response instead.", tk.CreatedBy), nil
	}

	// Block closing if there are open or awaiting_close sub-tickets
	var unclosedSubs []*protocol.Ticket
	for _, st := range []protocol.TicketStatus{protocol.TicketOpen, protocol.TicketAwaitingClose} {
		s := st
		subs, err := t.Broker.ListTickets(ticket.Filter{ParentID: ticketID, Status: &s})
		if err != nil {
			return "", fmt.Errorf("close_ticket: failed to check sub-tickets: %w", err)
		}
		unclosedSubs = append(unclosedSubs, subs...)
	}
	if len(unclosedSubs) > 0 {
		var ids []string
		for _, s := range unclosedSubs {
			ids = append(ids, fmt.Sprintf("%s (%s) [%s]", s.ID, s.Title, s.Status))
		}
		return "", fmt.Errorf("close_ticket: cannot close — %d unclosed sub-ticket(s) remain: %s. Use wait to wait for them to resolve.", len(unclosedSubs), strings.Join(ids, ", "))
	}

	if err := t.Broker.CloseTicket(ticketID, summary); err != nil {
		return "", fmt.Errorf("close_ticket: %w", err)
	}

	return fmt.Sprintf("Ticket %s closed: %s", ticketID, summary), nil
}

// --- SearchTicketsTool ---

type SearchTicketsTool struct {
	Broker  TicketBroker
	AgentID string
}

func (t *SearchTicketsTool) Name() string { return "search_tickets" }
func (t *SearchTicketsTool) Description() string {
	return "Search through tickets intentionally. Returns ticket count and compact summaries. Use get_ticket to read full details of a specific ticket."
}
func (t *SearchTicketsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":       map[string]any{"type": "string", "description": "Text search on ticket title and summary"},
			"status":      map[string]any{"type": "string", "enum": []string{"open", "awaiting_close", "closed"}, "description": "Filter by ticket status"},
			"participant": map[string]any{"type": "string", "description": "Filter by agent ID (created_by or assigned to)"},
			"limit":       map[string]any{"type": "integer", "description": "Max results to return (default 20)"},
		},
	}
}

func (t *SearchTicketsTool) Execute(_ context.Context, params map[string]any) (string, error) {
	filter := ticket.Filter{}

	if status := getString(params, "status"); status != "" {
		s := protocol.TicketStatus(status)
		filter.Status = &s
	}
	if participant := getString(params, "participant"); participant != "" {
		filter.AgentID = participant
	}
	if query := getString(params, "query"); query != "" {
		filter.Query = query
	}

	limit := 20
	if l, ok := params["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	filter.Limit = limit

	// Get total count (without limit)
	countFilter := filter
	countFilter.Limit = 0
	total, err := t.Broker.CountTickets(countFilter)
	if err != nil {
		return "", fmt.Errorf("search_tickets: count: %w", err)
	}

	tickets, err := t.Broker.ListTickets(filter)
	if err != nil {
		return "", fmt.Errorf("search_tickets: %w", err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Found %d ticket(s)", total)
	if total > limit {
		fmt.Fprintf(&b, " (showing %d)", limit)
	}
	b.WriteString("\n\n")

	if len(tickets) == 0 {
		b.WriteString("No tickets match your search.")
		return b.String(), nil
	}

	for _, tk := range tickets {
		fmt.Fprintf(&b, "- **%s** [%s] %s\n", tk.ID, tk.Status, tk.Title)
		fmt.Fprintf(&b, "  from: %s, assigned: %s, created: %s",
			tk.CreatedBy, strings.Join(tk.WaitingOn, ","), tk.CreatedAt.Format("2006-01-02 15:04"))
		if tk.Summary != "" {
			fmt.Fprintf(&b, "\n  summary: %s", tk.Summary)
		}
		b.WriteString("\n")
	}

	return b.String(), nil
}

// --- GetTicketTool ---

type GetTicketTool struct {
	Broker TicketBroker
}

func (t *GetTicketTool) Name() string        { return "get_ticket" }
func (t *GetTicketTool) Description() string  { return "Get full ticket details including messages" }
func (t *GetTicketTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"ticket_id": map[string]any{"type": "string", "description": "Ticket ID"},
		},
		"required": []string{"ticket_id"},
	}
}

func (t *GetTicketTool) Execute(_ context.Context, params map[string]any) (string, error) {
	ticketID := getString(params, "ticket_id")
	if ticketID == "" {
		return "", fmt.Errorf("get_ticket: ticket_id is required")
	}

	tk, err := t.Broker.GetTicket(ticketID)
	if err != nil {
		return "", fmt.Errorf("get_ticket: %w", err)
	}

	data, _ := json.MarshalIndent(tk, "", "  ")
	return string(data), nil
}

// --- WaitTool ---

// WaitTool lets an agent pause without sending a response. The agent will be
// woken when a sub-ticket resolves or a new message arrives on the ticket.
type WaitTool struct{}

func (t *WaitTool) Name() string        { return "wait" }
func (t *WaitTool) Description() string  { return "Stop processing and wait. Use after create_ticket to wait for sub-ticket results before responding." }
func (t *WaitTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *WaitTool) Execute(ctx context.Context, _ map[string]any) (string, error) {
	markResponded(ctx)
	return "Waiting. You will be woken when a sub-ticket resolves or a new message arrives.", nil
}

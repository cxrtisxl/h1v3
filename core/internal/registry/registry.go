package registry

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/h1v3-io/h1v3/internal/agent"
	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

const defaultInboxSize = 64

// Sink receives messages for a non-agent participant (e.g. _external → Telegram).
type Sink interface {
	Deliver(msg protocol.Message) error
}

// AgentHandle wraps a running agent with its inbox channel.
type AgentHandle struct {
	Spec  protocol.AgentSpec
	Agent *agent.Agent
	Inbox chan protocol.Message
}

// Registry is the central ticket broker that routes messages between agents.
type Registry struct {
	mu       sync.RWMutex
	store    ticket.Store
	agents   map[string]*AgentHandle
	sinks    map[string]Sink
	creators map[string]string // agent_id → creator_agent_id
	logger   *slog.Logger
}

// New creates a new Registry backed by the given ticket store.
func New(store ticket.Store, logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{
		store:    store,
		agents:   make(map[string]*AgentHandle),
		sinks:    make(map[string]Sink),
		creators: make(map[string]string),
		logger:   logger,
	}
}

// RegisterAgent adds an agent to the registry.
func (r *Registry) RegisterAgent(spec protocol.AgentSpec, ag *agent.Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[spec.ID]; exists {
		return fmt.Errorf("registry: agent %q already registered", spec.ID)
	}

	r.agents[spec.ID] = &AgentHandle{
		Spec:  spec,
		Agent: ag,
		Inbox: make(chan protocol.Message, defaultInboxSize),
	}
	r.logger.Info("agent registered", "agent", spec.ID)
	return nil
}

// DeregisterAgent removes an agent and closes its inbox.
func (r *Registry) DeregisterAgent(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	h, exists := r.agents[agentID]
	if !exists {
		return fmt.Errorf("registry: agent %q not found", agentID)
	}
	close(h.Inbox)
	delete(r.agents, agentID)
	r.logger.Info("agent deregistered", "agent", agentID)
	return nil
}

// RegisterSink registers a named sink for message delivery.
func (r *Registry) RegisterSink(name string, sink Sink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sinks[name] = sink
	r.logger.Info("sink registered", "name", name)
}

// GetAgent returns an agent handle by ID.
func (r *Registry) GetAgent(agentID string) (*AgentHandle, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.agents[agentID]
	return h, ok
}

// ListAgents returns all registered agent IDs.
func (r *Registry) ListAgents() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// CreateTicket creates a new ticket and routes an initial message to target agents.
func (r *Registry) CreateTicket(from, title, goal, parentID string, to []string, tags []string) (*protocol.Ticket, error) {
	now := time.Now()
	t := &protocol.Ticket{
		ID:        generateID(),
		Title:     title,
		Goal:      goal,
		Status:    protocol.TicketOpen,
		CreatedBy: from,
		WaitingOn: to,
		Tags:      tags,
		ParentID:  parentID,
		CreatedAt: now,
	}

	if err := r.store.Save(t); err != nil {
		return nil, fmt.Errorf("registry: create ticket: %w", err)
	}

	r.logger.Info("ticket created", "ticket", t.ID, "from", from, "to", to, "title", title)
	return t, nil
}

// RouteMessage persists a message to the ticket and delivers it to target agents' inboxes.
// Messages on closed tickets are persisted but NOT delivered to agent inboxes.
func (r *Registry) RouteMessage(msg protocol.Message) error {
	if msg.TicketID == "" {
		return fmt.Errorf("registry: message must have a ticket_id")
	}
	if msg.ID == "" {
		msg.ID = generateID()
	}

	// Check ticket status — don't deliver messages on closed tickets
	tk, err := r.store.Get(msg.TicketID)
	if err != nil {
		return fmt.Errorf("registry: route message: ticket lookup: %w", err)
	}
	// Persist message
	if err := r.store.AppendMessage(msg.TicketID, msg); err != nil {
		return fmt.Errorf("registry: route message: %w", err)
	}

	// Skip inbox delivery on closed tickets (message is still persisted for history)
	if tk.Status == protocol.TicketClosed {
		r.logger.Debug("ticket closed, message persisted but delivery skipped", "ticket", msg.TicketID, "from", msg.From)
		return nil
	}

	// Deliver to target agents
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, target := range msg.To {
		if h, ok := r.agents[target]; ok {
			select {
			case h.Inbox <- msg:
				r.logger.Debug("message delivered", "to", target, "ticket", msg.TicketID)
			default:
				r.logger.Warn("agent inbox full, dropping message", "agent", target, "ticket", msg.TicketID)
			}
			continue
		}
		if s, ok := r.sinks[target]; ok {
			if err := s.Deliver(msg); err != nil {
				r.logger.Error("sink delivery failed", "sink", target, "ticket", msg.TicketID, "error", err)
			} else {
				r.logger.Debug("message delivered to sink", "sink", target, "ticket", msg.TicketID)
			}
			continue
		}
		r.logger.Warn("target not found", "target", target, "ticket", msg.TicketID)
	}

	return nil
}

// PersistMessage saves a message to the ticket store without routing to agent inboxes.
func (r *Registry) PersistMessage(ticketID string, msg protocol.Message) error {
	if msg.ID == "" {
		msg.ID = generateID()
	}
	if err := r.store.AppendMessage(ticketID, msg); err != nil {
		return fmt.Errorf("registry: persist message: %w", err)
	}
	return nil
}

// CloseTicket marks a ticket as closed with a summary.
// If the ticket has a parent, a summary message is injected into the parent
// ticket and routed to the child ticket's creator so it can continue working
// on the parent task.
func (r *Registry) CloseTicket(ticketID, summary string) error {
	// Load ticket before closing to get parent info
	tk, err := r.store.Get(ticketID)
	if err != nil {
		return fmt.Errorf("registry: close ticket: %w", err)
	}

	// Idempotent: if already closed, skip (prevents duplicate relays)
	if tk.Status == protocol.TicketClosed {
		r.logger.Debug("ticket already closed, skipping", "ticket", ticketID)
		return nil
	}

	if err := r.store.Close(ticketID, summary); err != nil {
		return fmt.Errorf("registry: close ticket: %w", err)
	}
	r.logger.Info("ticket closed", "ticket", ticketID)

	// If child ticket, relay summary to parent
	if tk.ParentID != "" {
		r.relayToParent(tk, summary)
	}

	return nil
}

// relayToParent injects the child ticket's full conversation into the parent
// ticket, waking the creator agent in the parent context.
func (r *Registry) relayToParent(child *protocol.Ticket, summary string) {
	var b strings.Builder
	fmt.Fprintf(&b, "[Sub-ticket resolved: %q]\n", child.Title)
	fmt.Fprintf(&b, "Summary: %s\n", summary)
	if len(child.Messages) > 0 {
		b.WriteString("\nFull conversation:\n")
		for _, m := range child.Messages {
			fmt.Fprintf(&b, "[%s]: %s\n", m.From, m.Content)
		}
	}
	content := b.String()

	msg := protocol.Message{
		ID:        generateID(),
		From:      "_system",
		To:        []string{child.CreatedBy},
		Content:   content,
		TicketID:  child.ParentID,
		Timestamp: time.Now(),
	}

	if err := r.RouteMessage(msg); err != nil {
		r.logger.Error("failed to relay to parent ticket",
			"child", child.ID,
			"parent", child.ParentID,
			"error", err,
		)
	} else {
		r.logger.Info("relayed child summary to parent",
			"child", child.ID,
			"parent", child.ParentID,
			"creator", child.CreatedBy,
		)
	}
}

// GetTicket retrieves a ticket by ID.
func (r *Registry) GetTicket(ticketID string) (*protocol.Ticket, error) {
	return r.store.Get(ticketID)
}

// ListTickets returns tickets matching the filter.
func (r *Registry) ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error) {
	return r.store.List(filter)
}

// CountTickets returns the number of tickets matching the filter.
func (r *Registry) CountTickets(filter ticket.Filter) (int, error) {
	return r.store.Count(filter)
}

// ListSubTickets returns tickets whose parent_id matches the given ID.
func (r *Registry) ListSubTickets(parentID string) ([]*protocol.Ticket, error) {
	return r.store.List(ticket.Filter{ParentID: parentID})
}

// Store returns the underlying ticket store.
func (r *Registry) Store() ticket.Store {
	return r.store
}

package tool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// testBroker implements TicketBroker using a real SQLite ticket store.
type testBroker struct {
	store    ticket.Store
	messages []protocol.Message // track RouteMessage calls
}

func newTestBroker(t *testing.T) *testBroker {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := ticket.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { store.DB().Close() })
	return &testBroker{store: store}
}

func (b *testBroker) CreateTicket(from, title, goal, parentID string, to, tags []string) (*protocol.Ticket, error) {
	tk := &protocol.Ticket{
		ID:        fmt.Sprintf("tk-%d", len(b.messages)+1),
		Title:     title,
		Goal:      goal,
		Status:    protocol.TicketOpen,
		CreatedBy: from,
		WaitingOn: to,
		Tags:      tags,
		ParentID:  parentID,
	}
	if err := b.store.Save(tk); err != nil {
		return nil, err
	}
	return tk, nil
}

func (b *testBroker) GetTicket(id string) (*protocol.Ticket, error) {
	return b.store.Get(id)
}

func (b *testBroker) ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error) {
	return b.store.List(filter)
}

func (b *testBroker) CountTickets(filter ticket.Filter) (int, error) {
	return b.store.Count(filter)
}

func (b *testBroker) CloseTicket(id, summary string) error {
	return b.store.Close(id, summary)
}

func (b *testBroker) UpdateTicketStatus(ticketID string, status protocol.TicketStatus) error {
	return b.store.UpdateStatus(ticketID, status)
}

func (b *testBroker) RouteMessage(msg protocol.Message) error {
	b.messages = append(b.messages, msg)
	return b.store.AppendMessage(msg.TicketID, msg)
}

// --- Tests ---

func TestCreateTicketTool_Success(t *testing.T) {
	broker := newTestBroker(t)
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}

	result, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Test task",
		"goal":  "Get task completed",
		"tags":  []any{"test"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Ticket created") {
		t.Errorf("expected creation message, got %q", result)
	}
	if len(broker.messages) != 1 {
		t.Errorf("expected 1 routed message, got %d", len(broker.messages))
	}
}

func TestCreateTicketTool_MissingTitle(t *testing.T) {
	broker := newTestBroker(t)
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}

	_, err := ct.Execute(context.Background(), map[string]any{
		"to":   []any{"agent-b"},
		"goal": "some goal",
	})
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestCreateTicketTool_MissingGoal(t *testing.T) {
	broker := newTestBroker(t)
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}

	_, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "test",
	})
	if err == nil {
		t.Fatal("expected error for missing goal")
	}
}

func TestCreateTicketTool_NoTargets(t *testing.T) {
	broker := newTestBroker(t)
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}

	_, err := ct.Execute(context.Background(), map[string]any{
		"title": "test",
		"goal":  "some goal",
		"to":    []any{},
	})
	if err == nil {
		t.Fatal("expected error for empty targets")
	}
}

func TestRespondToTicketTool(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Respond test",
		"goal":  "Get a response",
	})
	ticketID := extractTicketID(result)

	rt := &RespondToTicketTool{Broker: broker, AgentID: "agent-b"}
	ctx := WithCurrentTicket(context.Background(), ticketID)
	resp, err := rt.Execute(ctx, map[string]any{
		"message": "Done!",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "Message sent") {
		t.Errorf("expected sent message, got %q", resp)
	}
}

func TestCloseTicketTool_Success(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Close test",
		"goal":  "Test closing",
	})
	ticketID := extractTicketID(result)

	closeTool := &CloseTicketTool{Broker: broker, AgentID: "agent-a"}
	resp, err := closeTool.Execute(context.Background(), map[string]any{
		"ticket_id": ticketID,
		"summary":   "All done",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "closed") {
		t.Errorf("expected 'closed' in response, got %q", resp)
	}
}

func TestCloseTicketTool_NonCreatorRejected(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Close reject test",
		"goal":  "Test rejection",
	})
	ticketID := extractTicketID(result)

	closeTool := &CloseTicketTool{Broker: broker, AgentID: "agent-b"}
	resp, err := closeTool.Execute(context.Background(), map[string]any{
		"ticket_id": ticketID,
		"summary":   "Trying to close",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "cannot close") {
		t.Errorf("expected guidance message, got %q", resp)
	}
	if !strings.Contains(resp, "respond_to_ticket") {
		t.Errorf("expected respond_to_ticket suggestion, got %q", resp)
	}
}

func TestGetTicketTool_Success(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Get test",
		"goal":  "Test get",
	})
	ticketID := extractTicketID(result)

	gt := &GetTicketTool{Broker: broker}
	resp, err := gt.Execute(context.Background(), map[string]any{"ticket_id": ticketID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "Get test") {
		t.Errorf("expected ticket title in JSON, got %q", resp)
	}
}

func TestCreateTicketTool_SubTicketSameRecipient_RequiresConfirmation(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent ticket: agent-a -> agent-b
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent task",
		"goal":  "Get something done",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := extractTicketID(result)

	// Agent-b tries to create a sub-ticket back to agent-a (same participant)
	ctB := &CreateTicketTool{Broker: broker, AgentID: "agent-b"}
	parentCtx := WithCurrentTicket(context.Background(), parentID)
	result, err = ctB.Execute(parentCtx, map[string]any{
		"to":    []any{"agent-a"},
		"title": "Sub task",
		"goal":  "Need more info",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "CONFIRMATION REQUIRED") {
		t.Errorf("expected confirmation prompt, got %q", result)
	}
	// No ticket should have been created
	if len(broker.messages) != 1 { // only the parent creation message
		t.Errorf("expected only 1 routed message (parent), got %d", len(broker.messages))
	}
}

func TestCreateTicketTool_SubTicketSameRecipient_ConfirmedProceeds(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent ticket: agent-a -> agent-b
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent task",
		"goal":  "Get something done",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := extractTicketID(result)

	// Agent-b creates a sub-ticket back to agent-a with confirmed=true
	ctB := &CreateTicketTool{Broker: broker, AgentID: "agent-b"}
	parentCtx := WithCurrentTicket(context.Background(), parentID)
	result, err = ctB.Execute(parentCtx, map[string]any{
		"to":        []any{"agent-a"},
		"title":     "Sub task",
		"goal":      "Need more info",
		"confirmed": true,
		"reason":    "Need clarification on a different aspect not covered by the parent ticket",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Ticket created") {
		t.Errorf("expected ticket creation, got %q", result)
	}
}

func TestCreateTicketTool_SubTicketConfirmedWithoutReason_Rejected(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent ticket: agent-a -> agent-b
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent task",
		"goal":  "Get something done",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := extractTicketID(result)

	// Agent-b tries confirmed=true but no reason
	ctB := &CreateTicketTool{Broker: broker, AgentID: "agent-b"}
	parentCtx := WithCurrentTicket(context.Background(), parentID)
	_, err = ctB.Execute(parentCtx, map[string]any{
		"to":        []any{"agent-a"},
		"title":     "Sub task",
		"goal":      "Need more info",
		"confirmed": true,
	})
	if err == nil {
		t.Fatal("expected error for confirmed without reason")
	}
	if !strings.Contains(err.Error(), "reason is required") {
		t.Errorf("expected 'reason is required' error, got: %v", err)
	}
}

func TestCreateTicketTool_SubTicketDifferentRecipient_NoConfirmation(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent ticket: agent-a -> agent-b
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, err := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent task",
		"goal":  "Get something done",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	parentID := extractTicketID(result)

	// Agent-b creates a sub-ticket to agent-c (different agent, no overlap)
	ctB := &CreateTicketTool{Broker: broker, AgentID: "agent-b"}
	parentCtx := WithCurrentTicket(context.Background(), parentID)
	result, err = ctB.Execute(parentCtx, map[string]any{
		"to":    []any{"agent-c"},
		"title": "Sub task to C",
		"goal":  "Delegate to someone else",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Ticket created") {
		t.Errorf("expected ticket creation without confirmation, got %q", result)
	}
}

func TestRespondToTicketTool_GoalMet_TransitionsToAwaitingClose(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Goal met test",
		"goal":  "Get a result",
	})
	ticketID := extractTicketID(result)

	rt := &RespondToTicketTool{Broker: broker, AgentID: "agent-b"}
	ctx := WithCurrentTicket(context.Background(), ticketID)
	resp, err := rt.Execute(ctx, map[string]any{
		"message":  "Here is the result.",
		"goal_met": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "awaiting_close") {
		t.Errorf("expected awaiting_close status note, got %q", resp)
	}

	tk, _ := broker.GetTicket(ticketID)
	if tk.Status != protocol.TicketAwaitingClose {
		t.Errorf("expected status awaiting_close, got %q", tk.Status)
	}
}

func TestRespondToTicketTool_GoalMet_RejectedForCreator(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Creator goal_met test",
		"goal":  "Test rejection",
	})
	ticketID := extractTicketID(result)

	// Creator tries to set goal_met
	rt := &RespondToTicketTool{Broker: broker, AgentID: "agent-a"}
	ctx := WithCurrentTicket(context.Background(), ticketID)
	_, err := rt.Execute(ctx, map[string]any{
		"message":  "I think it's done",
		"goal_met": true,
	})
	if err == nil {
		t.Fatal("expected error when creator sets goal_met")
	}
	if !strings.Contains(err.Error(), "only responders") {
		t.Errorf("expected 'only responders' error, got: %v", err)
	}
}

func TestRespondToTicketTool_CreatorRespond_ReopensAwaitingClose(t *testing.T) {
	broker := newTestBroker(t)

	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Reopen test",
		"goal":  "Get answer",
	})
	ticketID := extractTicketID(result)

	// Responder sets goal_met
	rt := &RespondToTicketTool{Broker: broker, AgentID: "agent-b"}
	ctx := WithCurrentTicket(context.Background(), ticketID)
	rt.Execute(ctx, map[string]any{
		"message":  "Done.",
		"goal_met": true,
	})

	// Creator responds — should reopen
	rtCreator := &RespondToTicketTool{Broker: broker, AgentID: "agent-a"}
	creatorCtx := WithCurrentTicket(context.Background(), ticketID)
	resp, err := rtCreator.Execute(creatorCtx, map[string]any{
		"message": "Not quite, need more detail.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resp, "open") {
		t.Errorf("expected open status note, got %q", resp)
	}

	tk, _ := broker.GetTicket(ticketID)
	if tk.Status != protocol.TicketOpen {
		t.Errorf("expected status open after creator respond, got %q", tk.Status)
	}
}

func TestCloseTicketTool_BlocksOnAwaitingCloseSubs(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent",
		"goal":  "Do stuff",
	})
	parentID := extractTicketID(result)

	// Create sub-ticket
	ctB := &CreateTicketTool{Broker: broker, AgentID: "agent-b"}
	subCtx := WithCurrentTicket(context.Background(), parentID)
	subResult, _ := ctB.Execute(subCtx, map[string]any{
		"to":    []any{"agent-c"},
		"title": "Sub task",
		"goal":  "Sub goal",
	})
	subID := extractTicketID(subResult)

	// Set sub-ticket to awaiting_close
	broker.UpdateTicketStatus(subID, protocol.TicketAwaitingClose)

	// Try to close parent — should fail
	closeTool := &CloseTicketTool{Broker: broker, AgentID: "agent-a"}
	_, err := closeTool.Execute(context.Background(), map[string]any{
		"ticket_id": parentID,
		"summary":   "Done",
	})
	if err == nil {
		t.Fatal("expected error when closing parent with awaiting_close sub-ticket")
	}
	if !strings.Contains(err.Error(), "unclosed sub-ticket") {
		t.Errorf("expected 'unclosed sub-ticket' error, got: %v", err)
	}
}

func TestCreateTicketTool_AwaitingCloseParent_RequiresConfirmation(t *testing.T) {
	broker := newTestBroker(t)

	// Create parent ticket
	ct := &CreateTicketTool{Broker: broker, AgentID: "agent-a"}
	result, _ := ct.Execute(context.Background(), map[string]any{
		"to":    []any{"agent-b"},
		"title": "Parent task",
		"goal":  "Get something done",
	})
	parentID := extractTicketID(result)

	// Set parent to awaiting_close
	broker.UpdateTicketStatus(parentID, protocol.TicketAwaitingClose)

	// Agent-a (creator) tries to create sub-ticket while parent is awaiting_close
	parentCtx := WithCurrentTicket(context.Background(), parentID)
	result, err := ct.Execute(parentCtx, map[string]any{
		"to":    []any{"agent-c"},
		"title": "New sub task",
		"goal":  "Need more",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "CONFIRMATION REQUIRED") {
		t.Errorf("expected confirmation prompt for awaiting_close parent, got %q", result)
	}
	if !strings.Contains(result, "awaiting_close") {
		t.Errorf("expected awaiting_close mention in prompt, got %q", result)
	}
}

// extractTicketID extracts the ticket ID from "Ticket created: <id> (title: ...)"
func extractTicketID(result string) string {
	parts := strings.SplitN(result, " ", 4)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

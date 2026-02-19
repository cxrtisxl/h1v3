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
	resp, err := rt.Execute(context.Background(), map[string]any{
		"ticket_id": ticketID,
		"message":   "Done!",
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
	_, err := closeTool.Execute(context.Background(), map[string]any{
		"ticket_id": ticketID,
		"summary":   "Trying to close",
	})
	if err == nil {
		t.Fatal("expected error when non-creator tries to close")
	}
	if !strings.Contains(err.Error(), "only the creator") {
		t.Errorf("expected 'only the creator' error, got: %v", err)
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

// extractTicketID extracts the ticket ID from "Ticket created: <id> (title: ...)"
func extractTicketID(result string) string {
	parts := strings.SplitN(result, " ", 4)
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

package registry

import (
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/h1v3-io/h1v3/internal/agent"
	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	store, err := ticket.NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t.Cleanup(func() { store.DB().Close() })
	return New(store, nil)
}

func dummyAgent(id string) (protocol.AgentSpec, *agent.Agent) {
	spec := protocol.AgentSpec{ID: id, CoreInstructions: "test"}
	// Agent needs a provider but for registry tests we don't call Run()
	a := &agent.Agent{
		Spec:  spec,
		Tools: tool.NewRegistry(),
	}
	return spec, a
}

func TestRegisterAndDeregister(t *testing.T) {
	r := newTestRegistry(t)

	spec, ag := dummyAgent("agent-a")
	if err := r.RegisterAgent(spec, ag); err != nil {
		t.Fatalf("register: %v", err)
	}

	ids := r.ListAgents()
	if len(ids) != 1 || ids[0] != "agent-a" {
		t.Errorf("expected [agent-a], got %v", ids)
	}

	// Duplicate registration
	if err := r.RegisterAgent(spec, ag); err == nil {
		t.Fatal("expected error for duplicate registration")
	}

	// Deregister
	if err := r.DeregisterAgent("agent-a"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if len(r.ListAgents()) != 0 {
		t.Error("expected no agents after deregister")
	}

	// Deregister nonexistent
	if err := r.DeregisterAgent("nonexistent"); err == nil {
		t.Fatal("expected error for deregistering nonexistent agent")
	}
}

func TestCreateTicket(t *testing.T) {
	r := newTestRegistry(t)

	tk, err := r.CreateTicket("agent-a", "Test ticket", "Verify creation", "", []string{"agent-b"}, []string{"test"})
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	if tk.ID == "" {
		t.Error("expected non-empty ticket ID")
	}
	if tk.Title != "Test ticket" {
		t.Errorf("expected title 'Test ticket', got %q", tk.Title)
	}
	if tk.CreatedBy != "agent-a" {
		t.Errorf("expected created_by 'agent-a', got %q", tk.CreatedBy)
	}

	// Verify persisted
	got, err := r.GetTicket(tk.ID)
	if err != nil {
		t.Fatalf("get ticket: %v", err)
	}
	if got.Title != "Test ticket" {
		t.Errorf("expected persisted title, got %q", got.Title)
	}
}

func TestRouteMessage(t *testing.T) {
	r := newTestRegistry(t)

	spec, ag := dummyAgent("agent-b")
	r.RegisterAgent(spec, ag)

	tk, _ := r.CreateTicket("agent-a", "Route test", "", "", []string{"agent-b"}, nil)

	msg := protocol.Message{
		ID:        "m-001",
		From:      "agent-a",
		To:        []string{"agent-b"},
		Content:   "Hello agent-b",
		TicketID:  tk.ID,
		Timestamp: time.Now(),
	}

	if err := r.RouteMessage(msg); err != nil {
		t.Fatalf("route: %v", err)
	}

	// Check inbox
	h, _ := r.GetAgent("agent-b")
	select {
	case received := <-h.Inbox:
		if received.Content != "Hello agent-b" {
			t.Errorf("expected 'Hello agent-b', got %q", received.Content)
		}
	default:
		t.Fatal("expected message in inbox")
	}

	// Verify persisted in ticket
	got, _ := r.GetTicket(tk.ID)
	if len(got.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got.Messages))
	}
}

func TestRouteMessage_NoTicketID(t *testing.T) {
	r := newTestRegistry(t)
	err := r.RouteMessage(protocol.Message{Content: "no ticket"})
	if err == nil {
		t.Fatal("expected error for message without ticket_id")
	}
}

func TestCloseTicket(t *testing.T) {
	r := newTestRegistry(t)

	tk, _ := r.CreateTicket("agent-a", "Close test", "", "", nil, nil)
	if err := r.CloseTicket(tk.ID, "All done"); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, _ := r.GetTicket(tk.ID)
	if got.Status != protocol.TicketClosed {
		t.Errorf("expected closed, got %q", got.Status)
	}
	if got.Summary != "All done" {
		t.Errorf("expected summary 'All done', got %q", got.Summary)
	}
}


// mockSink implements Sink for testing.
type mockSink struct {
	mu       sync.Mutex
	messages []protocol.Message
}

func (s *mockSink) Deliver(msg protocol.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
	return nil
}

func (s *mockSink) getMessages() []protocol.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]protocol.Message, len(s.messages))
	copy(cp, s.messages)
	return cp
}

func TestRouteMessage_ToSink(t *testing.T) {
	r := newTestRegistry(t)

	sink := &mockSink{}
	r.RegisterSink("_external", sink)

	tk, _ := r.CreateTicket("front", "Sink test", "", "", []string{"_external"}, nil)

	msg := protocol.Message{
		ID:       "m-sink",
		From:     "front",
		To:       []string{"_external"},
		Content:  "Hello from the hive!",
		TicketID: tk.ID,
	}

	if err := r.RouteMessage(msg); err != nil {
		t.Fatalf("route: %v", err)
	}

	// Check sink received the message
	delivered := sink.getMessages()
	if len(delivered) != 1 {
		t.Fatalf("expected 1 delivered message, got %d", len(delivered))
	}
	if delivered[0].Content != "Hello from the hive!" {
		t.Errorf("expected 'Hello from the hive!', got %q", delivered[0].Content)
	}

	// Verify also persisted in ticket store
	got, _ := r.GetTicket(tk.ID)
	if len(got.Messages) != 1 {
		t.Fatalf("expected 1 persisted message, got %d", len(got.Messages))
	}
}

func TestRouteMessage_MixedTargets(t *testing.T) {
	r := newTestRegistry(t)

	// Register an agent and a sink
	spec, ag := dummyAgent("agent-d")
	r.RegisterAgent(spec, ag)

	sink := &mockSink{}
	r.RegisterSink("_external", sink)

	tk, _ := r.CreateTicket("system", "Mixed test", "", "", []string{"agent-d", "_external"}, nil)

	msg := protocol.Message{
		ID:       "m-mixed",
		From:     "system",
		To:       []string{"agent-d", "_external"},
		Content:  "Broadcast",
		TicketID: tk.ID,
	}

	if err := r.RouteMessage(msg); err != nil {
		t.Fatalf("route: %v", err)
	}

	// Agent should have message in inbox
	h, _ := r.GetAgent("agent-d")
	select {
	case received := <-h.Inbox:
		if received.Content != "Broadcast" {
			t.Errorf("expected 'Broadcast', got %q", received.Content)
		}
	default:
		t.Fatal("expected message in agent inbox")
	}

	// Sink should have received the message
	delivered := sink.getMessages()
	if len(delivered) != 1 {
		t.Fatalf("expected 1 sink message, got %d", len(delivered))
	}
}

func TestListTickets(t *testing.T) {
	r := newTestRegistry(t)

	r.CreateTicket("a", "Ticket 1", "", "", nil, nil)
	r.CreateTicket("b", "Ticket 2", "", "", nil, nil)

	tickets, err := r.ListTickets(ticket.Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tickets) != 2 {
		t.Errorf("expected 2 tickets, got %d", len(tickets))
	}
}

func TestCloseTicket_RelayToParent(t *testing.T) {
	r := newTestRegistry(t)

	// Register the agent that will receive the relay message
	spec, ag := dummyAgent("front")
	r.RegisterAgent(spec, ag)

	// Create parent ticket (external → front)
	parent, _ := r.CreateTicket("_external", "User question", "", "", []string{"front"}, nil)

	// Create child ticket (front → coder) with parent
	child, _ := r.CreateTicket("front", "Get the name", "Get agent display name", parent.ID, []string{"coder"}, nil)

	// Verify parent_id was persisted
	got, _ := r.GetTicket(child.ID)
	if got.ParentID != parent.ID {
		t.Errorf("expected parent_id %q, got %q", parent.ID, got.ParentID)
	}

	// Close child ticket — should relay summary to parent
	if err := r.CloseTicket(child.ID, "Name is Neo"); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Front should receive a message on the parent ticket
	h, _ := r.GetAgent("front")
	select {
	case received := <-h.Inbox:
		if received.TicketID != parent.ID {
			t.Errorf("expected message on parent ticket %q, got %q", parent.ID, received.TicketID)
		}
		if received.From != "_system" {
			t.Errorf("expected from '_system', got %q", received.From)
		}
		if !strings.Contains(received.Content, "Name is Neo") {
			t.Errorf("expected summary in content, got %q", received.Content)
		}
		if !strings.Contains(received.Content, "Get the name") {
			t.Errorf("expected child title in content, got %q", received.Content)
		}
	default:
		t.Fatal("expected relay message in front's inbox")
	}

	// Verify message was also persisted on parent ticket
	parentGot, _ := r.GetTicket(parent.ID)
	if len(parentGot.Messages) != 1 {
		t.Fatalf("expected 1 message on parent ticket, got %d", len(parentGot.Messages))
	}

	// Closing the same child again should be a no-op (no second relay)
	if err := r.CloseTicket(child.ID, "Name is Neo again"); err != nil {
		t.Fatalf("second close: %v", err)
	}
	select {
	case msg := <-h.Inbox:
		t.Fatalf("expected no second relay, got: %v", msg)
	default:
		// OK — idempotent, no duplicate relay
	}
}

func TestCloseTicket_NoParent_NoRelay(t *testing.T) {
	r := newTestRegistry(t)

	spec, ag := dummyAgent("front")
	r.RegisterAgent(spec, ag)

	// Create ticket without parent
	tk, _ := r.CreateTicket("front", "No parent", "", "", nil, nil)

	if err := r.CloseTicket(tk.ID, "Done"); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Front should NOT receive any relay message
	h, _ := r.GetAgent("front")
	select {
	case msg := <-h.Inbox:
		t.Fatalf("expected no message, got: %v", msg)
	default:
		// OK — no message
	}
}

package agent

import (
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// mockExternalRouter implements ExternalRouter for testing.
type mockExternalRouter struct {
	mu       sync.Mutex
	tickets  map[string]*protocol.Ticket
	messages map[string][]protocol.Message // ticketID → messages
	closed   map[string]string             // ticketID → summary
	nextID   int
}

func newMockExternalRouter() *mockExternalRouter {
	return &mockExternalRouter{
		tickets:  make(map[string]*protocol.Ticket),
		messages: make(map[string][]protocol.Message),
		closed:   make(map[string]string),
	}
}

func (r *mockExternalRouter) RouteMessage(msg protocol.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages[msg.TicketID] = append(r.messages[msg.TicketID], msg)
	return nil
}

func (r *mockExternalRouter) GetTicket(ticketID string) (*protocol.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[ticketID]
	if !ok {
		return &protocol.Ticket{ID: ticketID, Status: protocol.TicketOpen}, nil
	}
	cp := *t
	cp.Messages = append([]protocol.Message{}, r.messages[ticketID]...)
	return &cp, nil
}

func (r *mockExternalRouter) CreateTicket(from, title, goal, parentID string, to []string, tags []string) (*protocol.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextID++
	t := &protocol.Ticket{
		ID:        fmt.Sprintf("t-%03d", r.nextID),
		Title:     title,
		Goal:      goal,
		Status:    protocol.TicketOpen,
		CreatedBy: from,
		WaitingOn: to,
		Tags:      tags,
		ParentID:  parentID,
	}
	r.tickets[t.ID] = t
	return t, nil
}

func (r *mockExternalRouter) CloseTicket(ticketID, summary string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed[ticketID] = summary
	if t, ok := r.tickets[ticketID]; ok {
		t.Status = protocol.TicketClosed
	}
	return nil
}

func (r *mockExternalRouter) messageCount(ticketID string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages[ticketID])
}

func (r *mockExternalRouter) lastMessage(ticketID string) protocol.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	msgs := r.messages[ticketID]
	return msgs[len(msgs)-1]
}

func (r *mockExternalRouter) closedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.closed)
}

func newTestSessionManager() (*SessionManager, *mockExternalRouter) {
	router := newMockExternalRouter()
	sm := NewSessionManager("front", router, slog.Default())
	return sm, router
}

func TestSessionManager_HandleInbound(t *testing.T) {
	sm, router := newTestSessionManager()

	err := sm.HandleInbound("chat-123", "Hello bot!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify session was created
	ticketID, ok := sm.GetSession("chat-123")
	if !ok {
		t.Fatal("expected session to be created for chat-123")
	}

	// Verify message was routed
	if count := router.messageCount(ticketID); count != 1 {
		t.Errorf("expected 1 routed message, got %d", count)
	}

	msg := router.lastMessage(ticketID)
	if msg.From != "_external" {
		t.Errorf("expected from '_external', got %q", msg.From)
	}
	if len(msg.To) != 1 || msg.To[0] != "front" {
		t.Errorf("expected to ['front'], got %v", msg.To)
	}
	if msg.TicketID != ticketID {
		t.Errorf("expected ticket %q, got %q", ticketID, msg.TicketID)
	}
}

func TestSessionManager_SessionReuse(t *testing.T) {
	sm, router := newTestSessionManager()

	// First message
	if err := sm.HandleInbound("chat-456", "First message"); err != nil {
		t.Fatalf("first message error: %v", err)
	}
	ticketID1, _ := sm.GetSession("chat-456")

	// Second message — should reuse same ticket
	if err := sm.HandleInbound("chat-456", "Second message"); err != nil {
		t.Fatalf("second message error: %v", err)
	}
	ticketID2, _ := sm.GetSession("chat-456")

	if ticketID1 != ticketID2 {
		t.Errorf("expected same ticket ID, got %q and %q", ticketID1, ticketID2)
	}

	// 2 messages routed total
	if count := router.messageCount(ticketID1); count != 2 {
		t.Errorf("expected 2 routed messages, got %d", count)
	}
}

func TestSessionManager_CloseSession(t *testing.T) {
	sm, router := newTestSessionManager()

	// Create a session
	sm.HandleInbound("chat-789", "Hello")
	ticketID, _ := sm.GetSession("chat-789")

	// Close it
	sm.CloseSession("chat-789")

	// Verify ticket was closed
	if router.closedCount() != 1 {
		t.Errorf("expected 1 closed ticket, got %d", router.closedCount())
	}

	// Verify session was removed
	if _, ok := sm.GetSession("chat-789"); ok {
		t.Error("expected session to be removed after close")
	}

	// Next message creates new session
	sm.HandleInbound("chat-789", "New topic")
	newTicketID, _ := sm.GetSession("chat-789")

	if ticketID == newTicketID {
		t.Error("expected different ticket ID after close")
	}
}

func TestSessionManager_CloseSession_NoSession(t *testing.T) {
	sm, router := newTestSessionManager()

	// Close nonexistent session — should not panic or close tickets
	sm.CloseSession("chat-nope")

	if router.closedCount() != 0 {
		t.Errorf("expected 0 closed tickets, got %d", router.closedCount())
	}
}

func TestSessionManager_Callbacks(t *testing.T) {
	sm, _ := newTestSessionManager()

	var createdChatID, createdTicketID, closedChatID string
	sm.OnSessionCreated = func(chatID, ticketID string) {
		createdChatID = chatID
		createdTicketID = ticketID
	}
	sm.OnSessionClosed = func(chatID string) {
		closedChatID = chatID
	}

	sm.HandleInbound("chat-cb", "Hello")

	if createdChatID != "chat-cb" {
		t.Errorf("expected OnSessionCreated with chat-cb, got %q", createdChatID)
	}
	if createdTicketID == "" {
		t.Error("expected non-empty ticketID in OnSessionCreated callback")
	}

	sm.CloseSession("chat-cb")
	if closedChatID != "chat-cb" {
		t.Errorf("expected OnSessionClosed with chat-cb, got %q", closedChatID)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	if got := truncate("a very long string that exceeds", 10); got != "a very lon..." {
		t.Errorf("expected truncated string, got %q", got)
	}
}

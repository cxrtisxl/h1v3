package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/h1v3-io/h1v3/internal/tool"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// mockRouter implements MessageRouter for testing.
type mockRouter struct {
	mu       sync.Mutex
	messages []protocol.Message
	tickets  map[string]*protocol.Ticket
}

func newMockRouter() *mockRouter {
	return &mockRouter{
		tickets: make(map[string]*protocol.Ticket),
	}
}

func (r *mockRouter) RouteMessage(msg protocol.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
	return nil
}

func (r *mockRouter) GetTicket(ticketID string) (*protocol.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[ticketID]
	if !ok {
		return nil, fmt.Errorf("ticket %q not found", ticketID)
	}
	return t, nil
}

func (r *mockRouter) UpdateTicketStatus(ticketID string, status protocol.TicketStatus) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.tickets[ticketID]
	if !ok {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	t.Status = status
	return nil
}

func (r *mockRouter) ListSubTickets(parentID string) ([]*protocol.Ticket, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var subs []*protocol.Ticket
	for _, t := range r.tickets {
		if t.ParentID == parentID {
			subs = append(subs, t)
		}
	}
	return subs, nil
}

func (r *mockRouter) getMessages() []protocol.Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]protocol.Message, len(r.messages))
	copy(cp, r.messages)
	return cp
}

func TestWorker_PlainTextDropped(t *testing.T) {
	router := newMockRouter()

	incomingMsg := protocol.Message{
		ID:        "m-001",
		From:      "agent-a",
		To:        []string{"agent-b"},
		Content:   "Please process this task.",
		TicketID:  "t-001",
		Timestamp: time.Now(),
	}

	router.tickets["t-001"] = &protocol.Ticket{
		ID:        "t-001",
		Title:     "Test ticket",
		Status:    protocol.TicketOpen,
		CreatedBy: "agent-a",
		WaitingOn: []string{"agent-b"},
		Messages:  []protocol.Message{incomingMsg},
	}

	// Agent returns plain text without calling respond_to_ticket
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			{Content: "I received the message and processed it."},
		},
	}

	ag := &Agent{
		Spec:          protocol.AgentSpec{ID: "agent-b", CoreInstructions: "You are a helpful agent."},
		Provider:      prov,
		Tools:         tool.NewRegistry(),
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	inbox := make(chan protocol.Message, 10)
	worker := &Worker{
		Agent:  ag,
		Inbox:  inbox,
		Router: router,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Start(ctx)
	}()

	inbox <- incomingMsg

	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()

	// Plain text output is dropped — no messages should be routed
	msgs := router.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected 0 routed messages (plain text dropped), got %d", len(msgs))
	}
}

func TestWorker_EmptyResponse_SkipsRoute(t *testing.T) {
	router := newMockRouter()

	incomingMsg := protocol.Message{
		ID:       "m-002",
		From:     "agent-a",
		To:       []string{"agent-b"},
		Content:  "Do something via tool",
		TicketID: "t-002",
	}

	router.tickets["t-002"] = &protocol.Ticket{
		ID:        "t-002",
		Title:     "Tool test",
		Status:    protocol.TicketOpen,
		CreatedBy: "agent-a",
		WaitingOn: []string{"agent-b"},
		Messages:  []protocol.Message{incomingMsg},
	}

	// Agent returns empty (it responded via respond_to_ticket)
	prov := &mockProvider{
		responses: []*protocol.ChatResponse{
			{Content: ""},
		},
	}

	ag := &Agent{
		Spec:          protocol.AgentSpec{ID: "agent-b", CoreInstructions: "test"},
		Provider:      prov,
		Tools:         tool.NewRegistry(),
		Logger:        slog.Default(),
		MaxIterations: 10,
	}

	inbox := make(chan protocol.Message, 10)
	worker := &Worker{Agent: ag, Inbox: inbox, Router: router}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		worker.Start(ctx)
	}()

	inbox <- incomingMsg

	time.Sleep(200 * time.Millisecond)
	cancel()
	wg.Wait()

	msgs := router.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected 0 routed messages for empty response, got %d", len(msgs))
	}
}

func TestWorker_InboxClosed(t *testing.T) {
	router := newMockRouter()
	ag := &Agent{
		Spec:          protocol.AgentSpec{ID: "agent-x", CoreInstructions: "test"},
		Tools:         tool.NewRegistry(),
		Logger:        slog.Default(),
		MaxIterations: 5,
	}

	inbox := make(chan protocol.Message)
	worker := &Worker{Agent: ag, Inbox: inbox, Router: router}

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(context.Background())
	}()

	close(inbox)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error on inbox close, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after inbox close")
	}
}


func TestWorker_ContextCancelled(t *testing.T) {
	router := newMockRouter()
	ag := &Agent{
		Spec:          protocol.AgentSpec{ID: "agent-y", CoreInstructions: "test"},
		Tools:         tool.NewRegistry(),
		Logger:        slog.Default(),
		MaxIterations: 5,
	}

	inbox := make(chan protocol.Message, 10)
	worker := &Worker{Agent: ag, Inbox: inbox, Router: router}

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- worker.Start(ctx)
	}()

	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after context cancel")
	}
}

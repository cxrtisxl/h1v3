package ticket

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() { s.DB().Close() })
	return s
}

func TestSaveAndGet(t *testing.T) {
	s := newTestStore(t)

	ticket := &protocol.Ticket{
		ID:        "t-001",
		Title:     "Fix the bug",
		Status:    protocol.TicketOpen,
		CreatedBy: "agent-a",
		WaitingOn: []string{"agent-b"},
		Tags:      []string{"bug", "urgent"},
		CreatedAt: time.Now().Truncate(time.Second),
	}

	if err := s.Save(ticket); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.Get("t-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Fix the bug" {
		t.Errorf("expected title 'Fix the bug', got %q", got.Title)
	}
	if got.Status != protocol.TicketOpen {
		t.Errorf("expected status open, got %q", got.Status)
	}
	if len(got.WaitingOn) != 1 || got.WaitingOn[0] != "agent-b" {
		t.Errorf("expected waiting_on [agent-b], got %v", got.WaitingOn)
	}
	if len(got.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(got.Tags))
	}
}

func TestSave_Upsert(t *testing.T) {
	s := newTestStore(t)

	ticket := &protocol.Ticket{
		ID:        "t-002",
		Title:     "Original",
		Status:    protocol.TicketOpen,
		CreatedBy: "a",
		CreatedAt: time.Now().Truncate(time.Second),
	}
	s.Save(ticket)

	ticket.Title = "Updated"
	s.Save(ticket)

	got, _ := s.Get("t-002")
	if got.Title != "Updated" {
		t.Errorf("expected 'Updated', got %q", got.Title)
	}
}

func TestGetNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing ticket")
	}
}

func TestAppendMessage(t *testing.T) {
	s := newTestStore(t)

	ticket := &protocol.Ticket{
		ID: "t-003", Title: "Test", Status: protocol.TicketOpen,
		CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
	}
	s.Save(ticket)

	msg := protocol.Message{
		ID:        "m-001",
		From:      "agent-a",
		To:        []string{"agent-b"},
		Content:   "Hello",
		TicketID:  "t-003",
		Timestamp: time.Now().Truncate(time.Second),
	}
	if err := s.AppendMessage("t-003", msg); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, _ := s.Get("t-003")
	if len(got.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(got.Messages))
	}
	if got.Messages[0].Content != "Hello" {
		t.Errorf("expected 'Hello', got %q", got.Messages[0].Content)
	}
	if got.Messages[0].From != "agent-a" {
		t.Errorf("expected from 'agent-a', got %q", got.Messages[0].From)
	}
}

func TestUpdateStatus(t *testing.T) {
	s := newTestStore(t)

	ticket := &protocol.Ticket{
		ID: "t-004", Title: "Test", Status: protocol.TicketOpen,
		CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
	}
	s.Save(ticket)

	if err := s.UpdateStatus("t-004", protocol.TicketClosed); err != nil {
		t.Fatalf("update status: %v", err)
	}

	got, _ := s.Get("t-004")
	if got.Status != protocol.TicketClosed {
		t.Errorf("expected closed, got %q", got.Status)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	s := newTestStore(t)
	err := s.UpdateStatus("nonexistent", protocol.TicketClosed)
	if err == nil {
		t.Fatal("expected error for missing ticket")
	}
}

func TestClose(t *testing.T) {
	s := newTestStore(t)

	ticket := &protocol.Ticket{
		ID: "t-005", Title: "Test", Status: protocol.TicketOpen,
		CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
	}
	s.Save(ticket)

	if err := s.Close("t-005", "Done and dusted"); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, _ := s.Get("t-005")
	if got.Status != protocol.TicketClosed {
		t.Errorf("expected closed, got %q", got.Status)
	}
	if got.Summary != "Done and dusted" {
		t.Errorf("expected summary, got %q", got.Summary)
	}
	if got.ClosedAt == nil {
		t.Error("expected closed_at to be set")
	}
}

func TestList_All(t *testing.T) {
	s := newTestStore(t)

	for i := range 3 {
		s.Save(&protocol.Ticket{
			ID: fmt.Sprintf("t-%d", i), Title: fmt.Sprintf("T%d", i),
			Status: protocol.TicketOpen, CreatedBy: "a",
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute).Truncate(time.Second),
		})
	}

	tickets, err := s.List(Filter{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tickets) != 3 {
		t.Errorf("expected 3 tickets, got %d", len(tickets))
	}
}

func TestList_FilterByStatus(t *testing.T) {
	s := newTestStore(t)

	s.Save(&protocol.Ticket{
		ID: "t-open", Title: "Open", Status: protocol.TicketOpen,
		CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
	})
	s.Save(&protocol.Ticket{
		ID: "t-closed", Title: "Closed", Status: protocol.TicketClosed,
		CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
	})

	open := protocol.TicketOpen
	tickets, _ := s.List(Filter{Status: &open})
	if len(tickets) != 1 {
		t.Errorf("expected 1 open ticket, got %d", len(tickets))
	}
}

func TestList_Limit(t *testing.T) {
	s := newTestStore(t)

	for i := range 5 {
		s.Save(&protocol.Ticket{
			ID: fmt.Sprintf("t-%d", i), Title: "T", Status: protocol.TicketOpen,
			CreatedBy: "a", CreatedAt: time.Now().Truncate(time.Second),
		})
	}

	tickets, _ := s.List(Filter{Limit: 2})
	if len(tickets) != 2 {
		t.Errorf("expected 2 tickets, got %d", len(tickets))
	}
}

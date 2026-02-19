package memory

import (
	"context"
	"testing"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

type mockProvider struct {
	response string
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Chat(_ context.Context, _ protocol.ChatRequest) (*protocol.ChatResponse, error) {
	return &protocol.ChatResponse{Content: m.response}, nil
}

func TestConsolidate_Basic(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := &mockProvider{
		response: `{"project": "API uses UTC internally, convert on display layer"}`,
	}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{
		Title:  "Fix timezone bug in API",
		Status: protocol.TicketClosed,
		Messages: []protocol.Message{
			{From: "user", Content: "The timezone is wrong in the response"},
			{From: "coder", Content: "Fixed by converting to UTC internally"},
		},
	}

	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	got := store.Get("project")
	if got != "API uses UTC internally, convert on display layer" {
		t.Errorf("project scope = %q", got)
	}
}

func TestConsolidate_AppendsToExisting(t *testing.T) {
	store := NewStore(t.TempDir())
	store.Set("project", "Uses PostgreSQL with WAL mode")

	mp := &mockProvider{
		response: `{"project": "API uses UTC internally"}`,
	}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{
		Title:    "Fix timezone",
		Status:   protocol.TicketClosed,
		Messages: []protocol.Message{{From: "a", Content: "done"}},
	}

	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	got := store.Get("project")
	expected := "Uses PostgreSQL with WAL mode\n\nAPI uses UTC internally"
	if got != expected {
		t.Errorf("project scope = %q, want %q", got, expected)
	}
}

func TestConsolidate_EmptyResponse(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := &mockProvider{response: `{}`}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{
		Title:    "Trivial task",
		Status:   protocol.TicketClosed,
		Messages: []protocol.Message{{From: "a", Content: "ok"}},
	}

	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	if len(store.List()) != 0 {
		t.Errorf("expected no scopes, got %v", store.List())
	}
}

func TestConsolidate_NoMessages(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := &mockProvider{response: `should not be called`}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{Title: "Empty", Messages: nil}
	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}
}

func TestConsolidate_MultipleScopes(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := &mockProvider{
		response: `{"project": "Uses React 18", "preferences": "User prefers dark mode"}`,
	}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{
		Title:    "Setup project",
		Status:   protocol.TicketClosed,
		Messages: []protocol.Message{{From: "a", Content: "set up with React 18 and dark mode"}},
	}

	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	if got := store.Get("project"); got != "Uses React 18" {
		t.Errorf("project = %q", got)
	}
	if got := store.Get("preferences"); got != "User prefers dark mode" {
		t.Errorf("preferences = %q", got)
	}
}

func TestConsolidate_CodeFencedJSON(t *testing.T) {
	store := NewStore(t.TempDir())
	mp := &mockProvider{
		response: "```json\n{\"project\": \"important fact\"}\n```",
	}
	c := &Consolidator{Provider: mp, Model: "test"}

	ticket := &protocol.Ticket{
		Title:    "Test",
		Status:   protocol.TicketClosed,
		Messages: []protocol.Message{{From: "a", Content: "b"}},
	}

	err := c.Consolidate(context.Background(), ticket, store)
	if err != nil {
		t.Fatalf("Consolidate: %v", err)
	}

	if got := store.Get("project"); got != "important fact" {
		t.Errorf("project = %q", got)
	}
}

func TestParseConsolidationResponse_Invalid(t *testing.T) {
	_, err := parseConsolidationResponse("not json at all")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

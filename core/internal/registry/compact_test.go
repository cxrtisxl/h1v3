package registry

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// mockProvider returns a fixed summary for compaction tests.
type mockCompactProvider struct {
	summary string
	called  bool
}

func (m *mockCompactProvider) Name() string { return "mock" }
func (m *mockCompactProvider) Chat(_ context.Context, _ protocol.ChatRequest) (*protocol.ChatResponse, error) {
	m.called = true
	return &protocol.ChatResponse{Content: m.summary}, nil
}

func makeMessages(n int) []protocol.Message {
	msgs := make([]protocol.Message, n)
	for i := range msgs {
		msgs[i] = protocol.Message{
			From:      "agent-a",
			To:        []string{"agent-b"},
			Content:   strings.Repeat("word ", 100), // ~100 words = ~130 tokens each
			Timestamp: time.Now(),
		}
	}
	return msgs
}

func TestEstimateTokens(t *testing.T) {
	got := EstimateTokens("hello world foo bar")
	// 4 words × 1.3 = 5
	if got != 5 {
		t.Errorf("EstimateTokens = %d, want 5", got)
	}
}

func TestShouldCompact_Below(t *testing.T) {
	c := &Compactor{Threshold: 100000}
	ticket := &protocol.Ticket{Messages: makeMessages(3)}
	if c.ShouldCompact(ticket) {
		t.Error("should not compact below threshold")
	}
}

func TestShouldCompact_Above(t *testing.T) {
	c := &Compactor{Threshold: 100}
	ticket := &protocol.Ticket{Messages: makeMessages(10)}
	if !c.ShouldCompact(ticket) {
		t.Error("should compact above threshold")
	}
}

func TestShouldCompact_ZeroThreshold(t *testing.T) {
	c := &Compactor{Threshold: 0}
	ticket := &protocol.Ticket{Messages: makeMessages(100)}
	if c.ShouldCompact(ticket) {
		t.Error("zero threshold should never trigger compaction")
	}
}

func TestCompact(t *testing.T) {
	mp := &mockCompactProvider{summary: "Agents discussed project setup."}
	c := &Compactor{
		Provider:  mp,
		Threshold: 100,
		Keep:      2,
	}

	ticket := &protocol.Ticket{Messages: makeMessages(8)}
	// Tag last 2 messages so we can identify them
	ticket.Messages[6].Content = "recent-msg-6"
	ticket.Messages[7].Content = "recent-msg-7"

	err := c.Compact(context.Background(), ticket)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	if !mp.called {
		t.Error("expected provider to be called")
	}

	// Should have 3 messages: 1 summary + 2 kept
	if len(ticket.Messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(ticket.Messages))
	}

	// First should be summary
	if !strings.Contains(ticket.Messages[0].Content, "[Compacted:") {
		t.Errorf("first message should be compacted summary, got %q", ticket.Messages[0].Content)
	}
	if !strings.Contains(ticket.Messages[0].Content, "Agents discussed project setup.") {
		t.Errorf("summary content missing, got %q", ticket.Messages[0].Content)
	}

	// Recent messages should be preserved
	if ticket.Messages[1].Content != "recent-msg-6" {
		t.Errorf("msg[1] = %q, want recent-msg-6", ticket.Messages[1].Content)
	}
	if ticket.Messages[2].Content != "recent-msg-7" {
		t.Errorf("msg[2] = %q, want recent-msg-7", ticket.Messages[2].Content)
	}
}

func TestCompact_TooFewMessages(t *testing.T) {
	mp := &mockCompactProvider{summary: "unused"}
	c := &Compactor{
		Provider:  mp,
		Threshold: 100,
		Keep:      4,
	}

	ticket := &protocol.Ticket{Messages: makeMessages(4)}
	err := c.Compact(context.Background(), ticket)
	if err != nil {
		t.Fatalf("Compact: %v", err)
	}

	if mp.called {
		t.Error("should not call provider when too few messages")
	}
	if len(ticket.Messages) != 4 {
		t.Errorf("messages should be unchanged, got %d", len(ticket.Messages))
	}
}

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// mockHiveService implements HiveService for testing.
type mockHiveService struct {
	agents   []AgentInfo
	tickets  []*protocol.Ticket
	injected []postMessageRequest
}

func (m *mockHiveService) ListAgents() []AgentInfo { return m.agents }
func (m *mockHiveService) GetAgent(id string) (*AgentInfo, bool) {
	for _, a := range m.agents {
		if a.ID == id {
			return &a, true
		}
	}
	return nil, false
}
func (m *mockHiveService) ListTickets(_ ticket.Filter) ([]*protocol.Ticket, error) {
	return m.tickets, nil
}
func (m *mockHiveService) GetTicket(id string) (*protocol.Ticket, error) {
	for _, t := range m.tickets {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (m *mockHiveService) InjectMessage(from, ticketID, content string) (string, error) {
	m.injected = append(m.injected, postMessageRequest{From: from, TicketID: ticketID, Content: content})
	if ticketID == "" {
		ticketID = "auto-ticket-1"
	}
	return ticketID, nil
}

func newTestServer(svc HiveService, key string) *Server {
	return NewServer(svc, Config{Host: "127.0.0.1", Port: 0, Key: key}, nil, nil)
}

func TestHealth(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "")
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var body map[string]string
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestListAgents(t *testing.T) {
	svc := &mockHiveService{
		agents: []AgentInfo{
			{ID: "coder", Role: "Developer"},
			{ID: "front", Role: "Front Agent"},
		},
	}
	srv := newTestServer(svc, "")
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
	var agents []AgentInfo
	json.NewDecoder(w.Body).Decode(&agents)
	if len(agents) != 2 {
		t.Errorf("got %d agents", len(agents))
	}
}

func TestGetAgent(t *testing.T) {
	svc := &mockHiveService{
		agents: []AgentInfo{{ID: "coder", Role: "Developer"}},
	}
	srv := newTestServer(svc, "")
	req := httptest.NewRequest("GET", "/api/agents/coder", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestGetAgent_NotFound(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "")
	req := httptest.NewRequest("GET", "/api/agents/ghost", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestListTickets(t *testing.T) {
	svc := &mockHiveService{
		tickets: []*protocol.Ticket{
			{ID: "t1", Title: "Task 1", Status: protocol.TicketOpen},
		},
	}
	srv := newTestServer(svc, "")
	req := httptest.NewRequest("GET", "/api/tickets?status=open&limit=10", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestGetTicket(t *testing.T) {
	svc := &mockHiveService{
		tickets: []*protocol.Ticket{{ID: "t1", Title: "Task 1"}},
	}
	srv := newTestServer(svc, "")
	req := httptest.NewRequest("GET", "/api/tickets/t1", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestGetTicket_NotFound(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "")
	req := httptest.NewRequest("GET", "/api/tickets/nope", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestPostMessage(t *testing.T) {
	svc := &mockHiveService{}
	srv := newTestServer(svc, "")
	body := `{"from":"user","ticket_id":"t1","content":"hello"}`
	req := httptest.NewRequest("POST", "/api/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202", w.Code)
	}
	if len(svc.injected) != 1 {
		t.Fatalf("expected 1 injected message, got %d", len(svc.injected))
	}
	if svc.injected[0].Content != "hello" {
		t.Errorf("content = %q", svc.injected[0].Content)
	}
}

func TestPostMessage_EmptyContent(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "")
	body := `{"from":"user","ticket_id":"t1","content":""}`
	req := httptest.NewRequest("POST", "/api/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestAuth_Required(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "secret-key")

	// No auth header
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no auth: status = %d, want 401", w.Code)
	}

	// Wrong key
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d, want 401", w.Code)
	}

	// Correct key
	req = httptest.NewRequest("GET", "/api/agents", nil)
	req.Header.Set("Authorization", "Bearer secret-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("correct key: status = %d, want 200", w.Code)
	}
}

func TestHealth_NoAuth(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "secret-key")
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Health should NOT require auth
	if w.Code != http.StatusOK {
		t.Errorf("health should not require auth, status = %d", w.Code)
	}
}

func TestCORS(t *testing.T) {
	srv := newTestServer(&mockHiveService{}, "")
	req := httptest.NewRequest("OPTIONS", "/api/agents", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("OPTIONS status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("CORS origin = %q", got)
	}
}

package webhook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/h1v3-io/h1v3/internal/connector"
)

type capturedMessage struct {
	mu   sync.Mutex
	msgs []connector.InboundMessage
}

func (c *capturedMessage) handler(_ context.Context, msg connector.InboundMessage) error {
	c.mu.Lock()
	c.msgs = append(c.msgs, msg)
	c.mu.Unlock()
	return nil
}

func (c *capturedMessage) last() connector.InboundMessage {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.msgs[len(c.msgs)-1]
}

func newTestHandler(endpoints map[string]EndpointConfig) (*Handler, *capturedMessage) {
	cap := &capturedMessage{}
	h := New(Config{Endpoints: endpoints}, cap.handler, nil)
	return h, cap
}

func TestWebhook_BasicPost(t *testing.T) {
	h, cap := newTestHandler(map[string]EndpointConfig{
		"github": {},
	})

	payload := `{"sender_id":"gh-bot","chat_id":"repo-123","content":"Deploy completed"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/github", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	msg := cap.last()
	if msg.Channel != "webhook:github" {
		t.Errorf("channel = %q", msg.Channel)
	}
	if msg.SenderID != "gh-bot" {
		t.Errorf("sender = %q", msg.SenderID)
	}
	if msg.Content != "Deploy completed" {
		t.Errorf("content = %q", msg.Content)
	}
}

func TestWebhook_BearerAuth(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{
		"ci": {BearerToken: "secret123"},
	})

	payload := `{"content":"build done"}`

	// Without auth
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/ci", strings.NewReader(payload))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", w.Code)
	}

	// With wrong auth
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/ci", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer wrong")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with wrong auth, got %d", w.Code)
	}

	// With correct auth
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/ci", strings.NewReader(payload))
	req.Header.Set("Authorization", "Bearer secret123")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with correct auth, got %d", w.Code)
	}
}

func TestWebhook_HMACAuth(t *testing.T) {
	secret := "webhook_secret_key"
	h, _ := newTestHandler(map[string]EndpointConfig{
		"github": {Secret: secret},
	})

	payload := []byte(`{"content":"push event"}`)
	sig := ComputeSignature(payload, secret)

	// With valid signature
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", sig)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid HMAC, got %d", w.Code)
	}

	// With invalid signature
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/github", strings.NewReader(string(payload)))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid HMAC, got %d", w.Code)
	}

	// Without signature
	req = httptest.NewRequest(http.MethodPost, "/api/webhook/github", strings.NewReader(string(payload)))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without signature, got %d", w.Code)
	}
}

func TestWebhook_UnknownEndpoint(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{
		"github": {},
	})

	req := httptest.NewRequest(http.MethodPost, "/api/webhook/unknown", strings.NewReader(`{"content":"hi"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown endpoint, got %d", w.Code)
	}
}

func TestWebhook_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{"test": {}})

	req := httptest.NewRequest(http.MethodGet, "/api/webhook/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestWebhook_EmptyContent(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{"test": {}})

	req := httptest.NewRequest(http.MethodPost, "/api/webhook/test", strings.NewReader(`{"content":""}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty content, got %d", w.Code)
	}
}

func TestWebhook_InvalidJSON(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{"test": {}})

	req := httptest.NewRequest(http.MethodPost, "/api/webhook/test", strings.NewReader(`not json`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestWebhook_Metadata(t *testing.T) {
	h, cap := newTestHandler(map[string]EndpointConfig{"ci": {}})

	payload := `{"content":"build done","metadata":{"build_id":"42","status":"success"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/ci", strings.NewReader(payload))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	msg := cap.last()
	if !strings.Contains(msg.Content, "build done") {
		t.Error("content missing main text")
	}
	if !strings.Contains(msg.Content, "build_id") {
		t.Error("content missing metadata")
	}
}

func TestWebhook_DefaultSenderAndChat(t *testing.T) {
	h, cap := newTestHandler(map[string]EndpointConfig{"test": {}})

	payload := `{"content":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhook/test", strings.NewReader(payload))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	msg := cap.last()
	if msg.SenderID != "test" {
		t.Errorf("default sender = %q, want %q", msg.SenderID, "test")
	}
	if msg.ChatID != "test" {
		t.Errorf("default chatID = %q, want %q", msg.ChatID, "test")
	}
}

func TestWebhook_ResponseBody(t *testing.T) {
	h, _ := newTestHandler(map[string]EndpointConfig{"test": {}})

	req := httptest.NewRequest(http.MethodPost, "/api/webhook/test", strings.NewReader(`{"content":"hi"}`))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("response status = %q", resp["status"])
	}
}

func TestExtractName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/api/webhook/github", "github"},
		{"/api/webhook/ci/", "ci"},
		{"/webhook", "webhook"},
		{"/", ""},
	}
	for _, tt := range tests {
		got := extractName(tt.path)
		if got != tt.want {
			t.Errorf("extractName(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

func TestComputeSignature(t *testing.T) {
	sig := ComputeSignature([]byte("test body"), "secret")
	if !strings.HasPrefix(sig, "sha256=") {
		t.Errorf("signature should start with sha256=: %q", sig)
	}
	// Verify it validates
	if !verifyHMAC([]byte("test body"), "secret", sig) {
		t.Error("signature should verify")
	}
}

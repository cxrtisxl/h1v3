package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/h1v3-io/h1v3/internal/connector"
)

// Config holds webhook connector configuration.
type Config struct {
	// Endpoints maps connector names to their auth secrets.
	// e.g., {"github": "whsec_abc123", "ci": "bearer_xyz"}
	Endpoints map[string]EndpointConfig `json:"endpoints"`
}

// EndpointConfig holds per-endpoint webhook configuration.
type EndpointConfig struct {
	// Secret for HMAC-SHA256 signature verification (X-Hub-Signature-256 header).
	// If empty, Bearer auth is used instead.
	Secret string `json:"secret,omitempty"`
	// BearerToken for Authorization header auth. Used if Secret is empty.
	BearerToken string `json:"bearer_token,omitempty"`
}

// WebhookPayload is the expected JSON body for webhook requests.
type WebhookPayload struct {
	SenderID string         `json:"sender_id"`
	ChatID   string         `json:"chat_id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Handler provides HTTP handlers for webhook endpoints.
type Handler struct {
	config  Config
	handler connector.InboundHandler
	logger  *slog.Logger
}

// New creates a new webhook handler.
func New(cfg Config, handler connector.InboundHandler, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		config:  cfg,
		handler: handler,
		logger:  logger,
	}
}

// ServeHTTP handles webhook requests at /api/webhook/{connector_name}.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract connector name from path: /api/webhook/{name}
	name := extractName(r.URL.Path)
	if name == "" {
		http.Error(w, "missing connector name in path", http.StatusBadRequest)
		return
	}

	endpoint, ok := h.config.Endpoints[name]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown webhook endpoint: %s", name), http.StatusNotFound)
		return
	}

	// Read body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// Authenticate
	if !h.authenticate(r, endpoint, body) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse payload
	var payload WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Content == "" {
		http.Error(w, "content is required", http.StatusBadRequest)
		return
	}

	// Build content with metadata if present
	content := payload.Content
	if len(payload.Metadata) > 0 {
		metaJSON, _ := json.Marshal(payload.Metadata)
		content = fmt.Sprintf("%s\n\n[Webhook metadata: %s]", content, string(metaJSON))
	}

	// Forward to inbound handler
	inbound := connector.InboundMessage{
		Channel:  "webhook:" + name,
		SenderID: payload.SenderID,
		ChatID:   payload.ChatID,
		Content:  content,
	}

	if payload.SenderID == "" {
		inbound.SenderID = name
	}
	if payload.ChatID == "" {
		inbound.ChatID = name
	}

	if err := h.handler(r.Context(), inbound); err != nil {
		h.logger.Error("webhook handler error",
			"endpoint", name,
			"error", err,
		)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handler) authenticate(r *http.Request, endpoint EndpointConfig, body []byte) bool {
	// HMAC signature verification
	if endpoint.Secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if sig == "" {
			sig = r.Header.Get("X-Signature-256")
		}
		return verifyHMAC(body, endpoint.Secret, sig)
	}

	// Bearer token
	if endpoint.BearerToken != "" {
		auth := r.Header.Get("Authorization")
		return auth == "Bearer "+endpoint.BearerToken
	}

	// No auth configured — allow (for development)
	return true
}

// verifyHMAC checks an HMAC-SHA256 signature.
// Signature format: "sha256=<hex>"
func verifyHMAC(body []byte, secret, signature string) bool {
	if signature == "" {
		return false
	}

	sig := strings.TrimPrefix(signature, "sha256=")
	expectedMAC, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	computedMAC := mac.Sum(nil)

	return hmac.Equal(computedMAC, expectedMAC)
}

// extractName gets the last path segment from /api/webhook/{name}.
func extractName(path string) string {
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// ComputeSignature generates an HMAC-SHA256 signature for testing/external use.
func ComputeSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

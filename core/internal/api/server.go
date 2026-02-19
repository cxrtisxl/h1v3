package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/h1v3-io/h1v3/internal/logbuf"
	"github.com/h1v3-io/h1v3/internal/ticket"
	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// LogQuerier abstracts log entry querying to avoid coupling to logbuf directly.
type LogQuerier interface {
	Query(since time.Time, minLevel slog.Level, limit int) []logbuf.Entry
}

// AgentInfo describes an agent for API responses.
type AgentInfo struct {
	ID   string `json:"id"`
	Role string `json:"role"`
}

// HiveService is the interface the API server needs from the hive.
type HiveService interface {
	ListAgents() []AgentInfo
	GetAgent(id string) (*AgentInfo, bool)
	ListTickets(filter ticket.Filter) ([]*protocol.Ticket, error)
	GetTicket(id string) (*protocol.Ticket, error)
	InjectMessage(from, ticketID, content string) (string, error) // returns ticket ID
}

// Config holds API server configuration.
type Config struct {
	Host string
	Port int
	Key  string // API key for Bearer auth
}

// Server is the h1v3 REST API server.
type Server struct {
	svc    HiveService
	cfg    Config
	logger *slog.Logger
	logs   LogQuerier
	srv    *http.Server
}

// NewServer creates a new API server. logs may be nil.
func NewServer(svc HiveService, cfg Config, logger *slog.Logger, logs LogQuerier) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		svc:    svc,
		cfg:    cfg,
		logger: logger,
		logs:   logs,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/agents", s.requireAuth(s.handleListAgents))
	mux.HandleFunc("GET /api/agents/{id}", s.requireAuth(s.handleGetAgent))
	mux.HandleFunc("GET /api/tickets", s.requireAuth(s.handleListTickets))
	mux.HandleFunc("GET /api/tickets/{id}", s.requireAuth(s.handleGetTicket))
	mux.HandleFunc("POST /api/messages", s.requireAuth(s.handlePostMessage))
	mux.HandleFunc("GET /api/logs", s.requireAuth(s.handleGetLogs))

	s.srv = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           s.corsMiddleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

// Start begins listening. Blocks until context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.srv.Shutdown(shutCtx)
	}()

	s.logger.Info("api server starting", "addr", s.srv.Addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("api server: %w", err)
	}
	return nil
}

// Handler returns the underlying http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.srv.Handler
}

// --- Middleware ---

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Key == "" {
			next(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != s.cfg.Key {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// --- Handlers ---

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleListAgents(w http.ResponseWriter, _ *http.Request) {
	agents := s.svc.ListAgents()
	writeJSON(w, http.StatusOK, agents)
}

func (s *Server) handleGetAgent(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	agent, ok := s.svc.GetAgent(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
		return
	}
	writeJSON(w, http.StatusOK, agent)
}

func (s *Server) handleListTickets(w http.ResponseWriter, r *http.Request) {
	filter := ticket.Filter{}
	if status := r.URL.Query().Get("status"); status != "" {
		ts := protocol.TicketStatus(status)
		filter.Status = &ts
	}
	if agent := r.URL.Query().Get("agent"); agent != "" {
		filter.AgentID = agent
	}
	if parentID := r.URL.Query().Get("parent_id"); parentID != "" {
		filter.ParentID = parentID
	}
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil {
			filter.Limit = n
		}
	}

	tickets, err := s.svc.ListTickets(filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, tickets)
}

func (s *Server) handleGetTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	t, err := s.svc.GetTicket(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ticket not found"})
		return
	}
	writeJSON(w, http.StatusOK, t)
}

type postMessageRequest struct {
	From     string `json:"from"`
	TicketID string `json:"ticket_id"`
	Content  string `json:"content"`
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var req postMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if req.Content == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "content is required"})
		return
	}

	ticketID, err := s.svc.InjectMessage(req.From, req.TicketID, req.Content)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted", "ticket_id": ticketID})
}

func (s *Server) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	if s.logs == nil {
		writeJSON(w, http.StatusOK, []logbuf.Entry{})
		return
	}

	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	minLevel := slog.LevelDebug
	if lvl := r.URL.Query().Get("level"); lvl != "" {
		switch strings.ToLower(lvl) {
		case "info":
			minLevel = slog.LevelInfo
		case "warn":
			minLevel = slog.LevelWarn
		case "error":
			minLevel = slog.LevelError
		}
	}

	var since time.Time
	if s := r.URL.Query().Get("since"); s != "" {
		if ms, err := strconv.ParseInt(s, 10, 64); err == nil {
			since = time.UnixMilli(ms)
		}
	}

	entries := s.logs.Query(since, minLevel, limit)
	if entries == nil {
		entries = []logbuf.Entry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

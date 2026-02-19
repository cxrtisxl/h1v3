package agent

import (
	"log/slog"
	"sync"
	"time"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// ExternalRouter provides ticket lifecycle and message routing for external sessions.
type ExternalRouter interface {
	RouteMessage(msg protocol.Message) error
	GetTicket(ticketID string) (*protocol.Ticket, error)
	CreateTicket(from, title, goal, parentID string, to []string, tags []string) (*protocol.Ticket, error)
	CloseTicket(ticketID, summary string) error
}

// SessionManager tracks external chat sessions and routes inbound messages
// to the front agent's inbox via RouteMessage (async — no inline LLM execution).
type SessionManager struct {
	FrontAgentID     string
	Router           ExternalRouter
	Logger           *slog.Logger
	OnSessionCreated func(chatID, ticketID string)
	OnSessionClosed  func(chatID string)

	mu       sync.Mutex
	sessions map[string]string // chatID → ticketID
}

// NewSessionManager creates a SessionManager for the given front agent.
func NewSessionManager(frontAgentID string, router ExternalRouter, logger *slog.Logger) *SessionManager {
	return &SessionManager{
		FrontAgentID: frontAgentID,
		Router:       router,
		Logger:       logger,
		sessions:     make(map[string]string),
	}
}

// HandleInbound routes an external message to the front agent's inbox.
// It returns immediately — the agent processes the message asynchronously.
func (sm *SessionManager) HandleInbound(chatID, content string) error {
	ticketID, err := sm.getOrCreateSession(chatID, content)
	if err != nil {
		return err
	}

	msg := protocol.Message{
		From:      "_external",
		To:        []string{sm.FrontAgentID},
		Content:   content,
		TicketID:  ticketID,
		Timestamp: time.Now(),
	}

	return sm.Router.RouteMessage(msg)
}

// CloseSession closes the active ticket for a chat and removes the session mapping.
func (sm *SessionManager) CloseSession(chatID string) {
	sm.mu.Lock()
	ticketID, ok := sm.sessions[chatID]
	if ok {
		delete(sm.sessions, chatID)
	}
	sm.mu.Unlock()

	if ok {
		if err := sm.Router.CloseTicket(ticketID, "session reset by user"); err != nil {
			sm.Logger.Error("failed to close ticket", "ticket", ticketID, "error", err)
		}
		if sm.OnSessionClosed != nil {
			sm.OnSessionClosed(chatID)
		}
	}
}

// GetSession returns the active ticket ID for a chat, if any.
func (sm *SessionManager) GetSession(chatID string) (string, bool) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	id, ok := sm.sessions[chatID]
	return id, ok
}

func (sm *SessionManager) getOrCreateSession(chatID, content string) (string, error) {
	sm.mu.Lock()
	ticketID, ok := sm.sessions[chatID]
	sm.mu.Unlock()

	if ok {
		return ticketID, nil
	}

	ticket, err := sm.Router.CreateTicket(
		"_external",
		truncate(content, 60),
		"",  // external sessions have no predefined goal
		"",  // no parent ticket
		[]string{sm.FrontAgentID},
		[]string{"external", "chat:" + chatID},
	)
	if err != nil {
		return "", err
	}

	sm.mu.Lock()
	sm.sessions[chatID] = ticket.ID
	sm.mu.Unlock()

	sm.Logger.Info("session created", "chat_id", chatID, "ticket", ticket.ID)

	if sm.OnSessionCreated != nil {
		sm.OnSessionCreated(chatID, ticket.ID)
	}

	return ticket.ID, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

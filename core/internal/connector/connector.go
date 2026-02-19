package connector

import "context"

// Connector is the interface for external messaging platforms (Telegram, Slack, etc.).
type Connector interface {
	// Name returns the connector type (e.g., "telegram", "slack").
	Name() string
	// Start begins listening for inbound messages. Blocks until context is cancelled.
	Start(ctx context.Context) error
	// Stop gracefully shuts down the connector.
	Stop() error
	// Send delivers an outbound message to the external platform.
	Send(ctx context.Context, msg OutboundMessage) error
}

// OutboundMessage is a message sent from the hive to an external platform.
type OutboundMessage struct {
	ChatID  string   // Platform-specific chat identifier
	Content string   // Message text (Markdown)
	Media   []string // Optional media file paths
}

// InboundMessage is a message received from an external platform.
type InboundMessage struct {
	Channel  string   // Connector name (e.g., "telegram")
	SenderID string   // Platform-specific sender identifier
	ChatID   string   // Platform-specific chat identifier
	Content  string   // Message text
	Media    []string // Downloaded media file paths
}

// InboundHandler processes messages received from external platforms.
// Implementations typically create or append to tickets via the Front Agent.
type InboundHandler func(ctx context.Context, msg InboundMessage) error

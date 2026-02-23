package protocol

import "time"

// TicketStatus represents the lifecycle state of a ticket.
type TicketStatus string

const (
	TicketOpen          TicketStatus = "open"
	TicketAwaitingClose TicketStatus = "awaiting_close"
	TicketClosed        TicketStatus = "closed"
)

// Ticket is an isolated chat context tied to a specific task.
type Ticket struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Goal      string       `json:"goal,omitempty"`
	Status    TicketStatus `json:"status"`
	CreatedBy string       `json:"created_by"`
	WaitingOn []string     `json:"waiting_on"`
	Messages  []Message    `json:"messages"`
	Tags      []string     `json:"tags,omitempty"`
	ParentID  string       `json:"parent_ticket_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	ClosedAt  *time.Time   `json:"closed_at,omitempty"`
	Summary   string       `json:"summary,omitempty"`
}

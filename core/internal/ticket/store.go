package ticket

import "github.com/h1v3-io/h1v3/pkg/protocol"

// Store is the persistence interface for tickets and their messages.
type Store interface {
	// Save creates or updates a ticket.
	Save(ticket *protocol.Ticket) error
	// Get retrieves a ticket by ID, including its messages.
	Get(id string) (*protocol.Ticket, error)
	// List returns tickets matching the filter.
	List(filter Filter) ([]*protocol.Ticket, error)
	// Count returns the number of tickets matching the filter.
	Count(filter Filter) (int, error)
	// AppendMessage adds a message to a ticket.
	AppendMessage(ticketID string, msg protocol.Message) error
	// UpdateStatus changes a ticket's status.
	UpdateStatus(ticketID string, status protocol.TicketStatus) error
	// Close marks a ticket as closed with a summary.
	Close(ticketID string, summary string) error
}

// Filter constrains ticket list queries.
type Filter struct {
	Status   *protocol.TicketStatus
	AgentID  string   // matches created_by or waiting_on
	Tags     []string // all must match
	Query    string   // text search on title and summary
	ParentID string   // exact match on parent_id
	Limit    int      // 0 = no limit
}

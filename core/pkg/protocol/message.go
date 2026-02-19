package protocol

import "time"

// Message is the fundamental unit of communication between agents.
type Message struct {
	ID        string    `json:"id"`
	From      string    `json:"from"`
	To        []string  `json:"to"`
	Content   string    `json:"content"`
	TicketID  string    `json:"ticket_id"`
	Timestamp time.Time `json:"timestamp"`
}

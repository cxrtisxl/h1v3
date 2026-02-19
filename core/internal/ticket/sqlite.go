package ticket

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite database and runs migrations.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("ticket store: open: %w", err)
	}

	// Enable WAL mode for better concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("ticket store: wal: %w", err)
	}

	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tickets (
			id         TEXT PRIMARY KEY,
			title      TEXT NOT NULL,
			goal       TEXT NOT NULL DEFAULT '',
			status     TEXT NOT NULL DEFAULT 'open',
			created_by TEXT NOT NULL,
			waiting_on TEXT NOT NULL DEFAULT '[]',
			tags       TEXT NOT NULL DEFAULT '[]',
			parent_id  TEXT NOT NULL DEFAULT '',
			summary    TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			closed_at  TEXT
		);

		CREATE TABLE IF NOT EXISTS ticket_messages (
			id        TEXT PRIMARY KEY,
			ticket_id TEXT NOT NULL REFERENCES tickets(id),
			sender    TEXT NOT NULL,
			recipients TEXT NOT NULL DEFAULT '[]',
			content   TEXT NOT NULL,
			timestamp TEXT NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_messages_ticket ON ticket_messages(ticket_id);
		CREATE INDEX IF NOT EXISTS idx_tickets_status ON tickets(status);
		CREATE INDEX IF NOT EXISTS idx_tickets_created_by ON tickets(created_by);
	`)
	if err != nil {
		return fmt.Errorf("ticket store: migrate: %w", err)
	}

	// Add columns to existing databases (idempotent).
	s.db.Exec(`ALTER TABLE tickets ADD COLUMN goal TEXT NOT NULL DEFAULT ''`)
	s.db.Exec(`ALTER TABLE tickets ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`)

	return nil
}

func (s *SQLiteStore) Save(t *protocol.Ticket) error {
	waitingOn, _ := json.Marshal(t.WaitingOn)
	tags, _ := json.Marshal(t.Tags)
	var closedAt *string
	if t.ClosedAt != nil {
		v := t.ClosedAt.Format(time.RFC3339)
		closedAt = &v
	}

	_, err := s.db.Exec(`
		INSERT INTO tickets (id, title, goal, status, created_by, waiting_on, tags, parent_id, summary, created_at, closed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title=excluded.title, goal=excluded.goal, status=excluded.status, waiting_on=excluded.waiting_on,
			tags=excluded.tags, parent_id=excluded.parent_id, summary=excluded.summary, closed_at=excluded.closed_at
	`, t.ID, t.Title, t.Goal, string(t.Status), t.CreatedBy, string(waitingOn), string(tags),
		t.ParentID, t.Summary, t.CreatedAt.Format(time.RFC3339), closedAt)
	if err != nil {
		return fmt.Errorf("ticket store: save: %w", err)
	}
	return nil
}

func (s *SQLiteStore) Get(id string) (*protocol.Ticket, error) {
	row := s.db.QueryRow(`SELECT id, title, goal, status, created_by, waiting_on, tags, parent_id, summary, created_at, closed_at FROM tickets WHERE id = ?`, id)

	t, err := scanTicket(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("ticket %q not found", id)
		}
		return nil, fmt.Errorf("ticket store: get: %w", err)
	}

	// Load messages
	msgs, err := s.loadMessages(id)
	if err != nil {
		return nil, err
	}
	t.Messages = msgs
	return t, nil
}

func (s *SQLiteStore) List(filter Filter) ([]*protocol.Ticket, error) {
	query := "SELECT id, title, goal, status, created_by, waiting_on, tags, parent_id, summary, created_at, closed_at FROM tickets WHERE 1=1"
	var args []any

	if filter.Status != nil {
		query += " AND status = ?"
		args = append(args, string(*filter.Status))
	}
	if filter.AgentID != "" {
		query += " AND (created_by = ? OR waiting_on LIKE ?)"
		args = append(args, filter.AgentID, fmt.Sprintf("%%%s%%", filter.AgentID))
	}
	if len(filter.Tags) > 0 {
		for _, tag := range filter.Tags {
			query += " AND tags LIKE ?"
			args = append(args, fmt.Sprintf("%%%s%%", tag))
		}
	}
	if filter.ParentID != "" {
		query += " AND parent_id = ?"
		args = append(args, filter.ParentID)
	}
	if filter.Query != "" {
		query += " AND (title LIKE ? OR summary LIKE ?)"
		pattern := fmt.Sprintf("%%%s%%", filter.Query)
		args = append(args, pattern, pattern)
	}
	query += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("ticket store: list: %w", err)
	}
	defer rows.Close()

	var tickets []*protocol.Ticket
	for rows.Next() {
		t, err := scanTicketRows(rows)
		if err != nil {
			return nil, fmt.Errorf("ticket store: list scan: %w", err)
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (s *SQLiteStore) Count(filter Filter) (int, error) {
	query := "SELECT COUNT(*) FROM tickets WHERE 1=1"
	var args []any

	if filter.Status != nil {
		query += " AND status = ?"
		args = append(args, string(*filter.Status))
	}
	if filter.AgentID != "" {
		query += " AND (created_by = ? OR waiting_on LIKE ?)"
		args = append(args, filter.AgentID, fmt.Sprintf("%%%s%%", filter.AgentID))
	}
	if len(filter.Tags) > 0 {
		for _, tag := range filter.Tags {
			query += " AND tags LIKE ?"
			args = append(args, fmt.Sprintf("%%%s%%", tag))
		}
	}
	if filter.ParentID != "" {
		query += " AND parent_id = ?"
		args = append(args, filter.ParentID)
	}
	if filter.Query != "" {
		query += " AND (title LIKE ? OR summary LIKE ?)"
		pattern := fmt.Sprintf("%%%s%%", filter.Query)
		args = append(args, pattern, pattern)
	}

	var count int
	err := s.db.QueryRow(query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("ticket store: count: %w", err)
	}
	return count, nil
}

func (s *SQLiteStore) AppendMessage(ticketID string, msg protocol.Message) error {
	recipients, _ := json.Marshal(msg.To)
	_, err := s.db.Exec(`INSERT INTO ticket_messages (id, ticket_id, sender, recipients, content, timestamp) VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, ticketID, msg.From, string(recipients), msg.Content, msg.Timestamp.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("ticket store: append message: %w", err)
	}
	return nil
}

func (s *SQLiteStore) UpdateStatus(ticketID string, status protocol.TicketStatus) error {
	result, err := s.db.Exec(`UPDATE tickets SET status = ? WHERE id = ?`, string(status), ticketID)
	if err != nil {
		return fmt.Errorf("ticket store: update status: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	return nil
}

func (s *SQLiteStore) Close(ticketID string, summary string) error {
	now := time.Now().Format(time.RFC3339)
	result, err := s.db.Exec(`UPDATE tickets SET status = 'closed', summary = ?, closed_at = ? WHERE id = ?`,
		summary, now, ticketID)
	if err != nil {
		return fmt.Errorf("ticket store: close: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("ticket %q not found", ticketID)
	}
	return nil
}

// DB returns the underlying database connection (for testing or direct access).
func (s *SQLiteStore) DB() *sql.DB {
	return s.db
}

// --- helpers ---

func (s *SQLiteStore) loadMessages(ticketID string) ([]protocol.Message, error) {
	rows, err := s.db.Query(`SELECT id, sender, recipients, content, timestamp FROM ticket_messages WHERE ticket_id = ? ORDER BY timestamp`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("ticket store: load messages: %w", err)
	}
	defer rows.Close()

	var msgs []protocol.Message
	for rows.Next() {
		var m protocol.Message
		var recipientsJSON, ts string
		if err := rows.Scan(&m.ID, &m.From, &recipientsJSON, &m.Content, &ts); err != nil {
			return nil, fmt.Errorf("ticket store: scan message: %w", err)
		}
		json.Unmarshal([]byte(recipientsJSON), &m.To)
		m.Timestamp, _ = time.Parse(time.RFC3339, ts)
		m.TicketID = ticketID
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanTicketFromRow(s scannable) (*protocol.Ticket, error) {
	var t protocol.Ticket
	var waitingOnJSON, tagsJSON, createdAtStr string
	var closedAtStr *string
	var status string

	err := s.Scan(&t.ID, &t.Title, &t.Goal, &status, &t.CreatedBy, &waitingOnJSON, &tagsJSON,
		&t.ParentID, &t.Summary, &createdAtStr, &closedAtStr)
	if err != nil {
		return nil, err
	}

	t.Status = protocol.TicketStatus(status)
	json.Unmarshal([]byte(waitingOnJSON), &t.WaitingOn)
	json.Unmarshal([]byte(tagsJSON), &t.Tags)
	t.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if closedAtStr != nil {
		ct, _ := time.Parse(time.RFC3339, *closedAtStr)
		t.ClosedAt = &ct
	}

	// Ensure nil slices are empty slices
	if t.WaitingOn == nil {
		t.WaitingOn = []string{}
	}
	if t.Tags == nil {
		t.Tags = []string{}
	}

	return &t, nil
}

func scanTicket(row *sql.Row) (*protocol.Ticket, error) {
	return scanTicketFromRow(row)
}

func scanTicketRows(rows *sql.Rows) (*protocol.Ticket, error) {
	return scanTicketFromRow(rows)
}

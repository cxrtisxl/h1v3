package logbuf

import (
	"log/slog"
	"sync"
	"time"
)

// Entry is a single log entry captured from slog.
type Entry struct {
	Time    time.Time      `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Attrs   map[string]any `json:"attrs,omitempty"`
}

// Buffer is a thread-safe ring buffer for log entries.
type Buffer struct {
	mu      sync.Mutex
	entries []Entry
	size    int
	pos     int
	count   int
}

// New creates a new ring buffer that holds up to size entries.
func New(size int) *Buffer {
	return &Buffer{
		entries: make([]Entry, size),
		size:    size,
	}
}

// Write appends an entry to the ring buffer.
func (b *Buffer) Write(e Entry) {
	b.mu.Lock()
	b.entries[b.pos] = e
	b.pos = (b.pos + 1) % b.size
	if b.count < b.size {
		b.count++
	}
	b.mu.Unlock()
}

// Query returns entries matching the given filters, oldest first.
// If since is zero, all entries are considered. If limit <= 0, all matching entries are returned.
func (b *Buffer) Query(since time.Time, minLevel slog.Level, limit int) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()

	var result []Entry

	// Walk the ring buffer oldest-first
	start := 0
	n := b.count
	if b.count == b.size {
		start = b.pos // oldest entry when buffer is full
	}

	for i := 0; i < n; i++ {
		idx := (start + i) % b.size
		e := b.entries[idx]

		if !since.IsZero() && e.Time.Before(since) {
			continue
		}
		if parseSlogLevel(e.Level) < minLevel {
			continue
		}
		result = append(result, e)
	}

	if limit > 0 && len(result) > limit {
		result = result[len(result)-limit:]
	}
	return result
}

// parseSlogLevel converts a level string back to slog.Level.
func parseSlogLevel(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

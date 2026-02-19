package logbuf

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func TestBufferWriteAndQuery(t *testing.T) {
	buf := New(5)
	now := time.Now()

	for i := 0; i < 3; i++ {
		buf.Write(Entry{
			Time:    now.Add(time.Duration(i) * time.Second),
			Level:   "INFO",
			Message: "msg",
		})
	}

	entries := buf.Query(time.Time{}, slog.LevelDebug, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
}

func TestBufferRingOverwrite(t *testing.T) {
	buf := New(3)
	now := time.Now()

	for i := 0; i < 5; i++ {
		buf.Write(Entry{
			Time:    now.Add(time.Duration(i) * time.Second),
			Level:   "INFO",
			Message: "msg",
			Attrs:   map[string]any{"i": i},
		})
	}

	entries := buf.Query(time.Time{}, slog.LevelDebug, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries (ring buffer size), got %d", len(entries))
	}
	// Should be entries 2, 3, 4 (oldest first)
	if entries[0].Attrs["i"] != 2 {
		t.Fatalf("expected first entry i=2, got %v", entries[0].Attrs["i"])
	}
	if entries[2].Attrs["i"] != 4 {
		t.Fatalf("expected last entry i=4, got %v", entries[2].Attrs["i"])
	}
}

func TestBufferQuerySince(t *testing.T) {
	buf := New(10)
	now := time.Now()

	for i := 0; i < 5; i++ {
		buf.Write(Entry{
			Time:    now.Add(time.Duration(i) * time.Second),
			Level:   "INFO",
			Message: "msg",
		})
	}

	since := now.Add(3 * time.Second)
	entries := buf.Query(since, slog.LevelDebug, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries since t+3s, got %d", len(entries))
	}
}

func TestBufferQueryLevel(t *testing.T) {
	buf := New(10)
	now := time.Now()

	buf.Write(Entry{Time: now, Level: "DEBUG", Message: "debug"})
	buf.Write(Entry{Time: now, Level: "INFO", Message: "info"})
	buf.Write(Entry{Time: now, Level: "WARN", Message: "warn"})
	buf.Write(Entry{Time: now, Level: "ERROR", Message: "error"})

	entries := buf.Query(time.Time{}, slog.LevelWarn, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries at WARN+, got %d", len(entries))
	}
	if entries[0].Message != "warn" || entries[1].Message != "error" {
		t.Fatalf("unexpected entries: %v", entries)
	}
}

func TestBufferQueryLimit(t *testing.T) {
	buf := New(10)
	now := time.Now()

	for i := 0; i < 8; i++ {
		buf.Write(Entry{Time: now.Add(time.Duration(i) * time.Second), Level: "INFO", Message: "msg"})
	}

	entries := buf.Query(time.Time{}, slog.LevelDebug, 3)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries with limit, got %d", len(entries))
	}
}

func TestHandlerCaptures(t *testing.T) {
	buf := New(10)
	inner := slog.NewTextHandler(&discardWriter{}, nil)
	handler := NewHandler(inner, buf)
	logger := slog.New(handler)

	logger.Info("hello", "key", "value")
	logger.Warn("warning")

	entries := buf.Query(time.Time{}, slog.LevelDebug, 0)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Fatalf("expected 'hello', got %q", entries[0].Message)
	}
	if entries[0].Attrs["key"] != "value" {
		t.Fatalf("expected attr key=value, got %v", entries[0].Attrs)
	}
	if entries[1].Level != "WARN" {
		t.Fatalf("expected WARN level, got %q", entries[1].Level)
	}
}

func TestHandlerWithAttrs(t *testing.T) {
	buf := New(10)
	inner := slog.NewTextHandler(&discardWriter{}, nil)
	handler := NewHandler(inner, buf)
	logger := slog.New(handler).With("component", "test")

	logger.Info("msg")

	entries := buf.Query(time.Time{}, slog.LevelDebug, 0)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Attrs["component"] != "test" {
		t.Fatalf("expected component=test, got %v", entries[0].Attrs)
	}
}

func TestHandlerEnabledAlwaysTrue(t *testing.T) {
	buf := New(10)
	inner := slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	handler := NewHandler(inner, buf)

	// Buffer handler always returns true so it captures all levels
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("expected DEBUG to be enabled (buffer captures all)")
	}
	if !handler.Enabled(context.Background(), slog.LevelWarn) {
		t.Fatal("expected WARN to be enabled")
	}
}

func TestHandlerCapturesAllLevels(t *testing.T) {
	buf := New(10)
	// Inner handler only allows WARN+
	inner := slog.NewTextHandler(&discardWriter{}, &slog.HandlerOptions{Level: slog.LevelWarn})
	handler := NewHandler(inner, buf)
	logger := slog.New(handler)

	logger.Debug("debug msg")
	logger.Info("info msg")
	logger.Warn("warn msg")

	// Buffer should have all 3 even though inner only allows WARN+
	entries := buf.Query(time.Time{}, slog.LevelDebug, 0)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries in buffer, got %d", len(entries))
	}
}

type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (int, error) { return len(p), nil }

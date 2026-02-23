package logbuf

import (
	"context"
	"log/slog"
)

// Handler is an slog.Handler that captures entries into a Buffer
// and delegates to an inner handler.
type Handler struct {
	inner  slog.Handler
	buf    *Buffer
	attrs  []slog.Attr
	groups []string
}

// NewHandler creates a handler that writes to both buf and inner.
func NewHandler(inner slog.Handler, buf *Buffer) *Handler {
	return &Handler{inner: inner, buf: buf}
}

func (h *Handler) Enabled(_ context.Context, _ slog.Level) bool {
	// Always return true so the buffer captures all log levels,
	// regardless of the inner handler's level filter.
	return true
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Collect attributes
	attrs := make(map[string]any)
	// Pre-bound attrs from WithAttrs
	for _, a := range h.attrs {
		key := a.Key
		for _, g := range h.groups {
			key = g + "." + key
		}
		attrs[key] = resolveAttrValue(a.Value)
	}
	// Record-level attrs
	r.Attrs(func(a slog.Attr) bool {
		key := a.Key
		for _, g := range h.groups {
			key = g + "." + key
		}
		attrs[key] = resolveAttrValue(a.Value)
		return true
	})

	var attrsMap map[string]any
	if len(attrs) > 0 {
		attrsMap = attrs
	}

	h.buf.Write(Entry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
		Attrs:   attrsMap,
	})

	// Only delegate to inner if it would handle this level
	// (so stdout respects its configured level filter).
	if h.inner.Enabled(ctx, r.Level) {
		return h.inner.Handle(ctx, r)
	}
	return nil
}

// resolveAttrValue converts slog values to JSON-safe types.
// Errors are converted to their string representation so they don't
// serialize to {} when JSON-marshaled.
func resolveAttrValue(v slog.Value) any {
	v = v.Resolve()
	raw := v.Any()
	if err, ok := raw.(error); ok {
		return err.Error()
	}
	return raw
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &Handler{
		inner:  h.inner.WithAttrs(attrs),
		buf:    h.buf,
		attrs:  append(h.attrs[:len(h.attrs):len(h.attrs)], attrs...),
		groups: h.groups,
	}
}

func (h *Handler) WithGroup(name string) slog.Handler {
	return &Handler{
		inner:  h.inner.WithGroup(name),
		buf:    h.buf,
		attrs:  h.attrs,
		groups: append(h.groups[:len(h.groups):len(h.groups)], name),
	}
}

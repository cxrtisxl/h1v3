package provider

import (
	"context"

	"github.com/h1v3-io/h1v3/pkg/protocol"
)

// Provider is the abstraction over LLM APIs.
type Provider interface {
	Chat(ctx context.Context, req protocol.ChatRequest) (*protocol.ChatResponse, error)
	Name() string
}

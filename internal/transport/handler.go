package transport

import (
	"context"
	"mcpwatch/internal/engine"
)

// Handler defines the interface for all transport interceptors.
// Each transport (stdio, sse, http, ebpf) implements this interface.
type Handler interface {
	// Start begins intercepting traffic. Parsed messages are sent to the channel.
	// Blocks until the transport is done or the context is cancelled.
	Start(ctx context.Context, messages chan<- *engine.Message) error

	// Type returns the transport identifier (stdio, sse, http, ebpf).
	Type() string
}

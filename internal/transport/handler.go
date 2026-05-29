package transport

import (
	"context"
	"mcpwatch/internal/engine"
)



type Handler interface {

	
	Start(ctx context.Context, messages chan<- *engine.Message) error

	
	Type() string
}

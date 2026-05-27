//go:build !linux

package transport

import (
	"context"
	"fmt"
	"mcpwatch/internal/engine"
)

// EBPFHandler intercepts JSON-RPC over stdin/stdout for an already running process using eBPF.
type EBPFHandler struct {
	pid int
}

func NewEBPF(pid int) *EBPFHandler {
	return &EBPFHandler{pid: pid}
}

func (h *EBPFHandler) Type() string {
	return "ebpf"
}

func (h *EBPFHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	return fmt.Errorf("eBPF interception is natively supported on Linux only. Please run this command on a Linux system or WSL2")
}

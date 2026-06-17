//go:build !linux

package transport

import (
	"context"
	"fmt"
	"mcpwatch/internal/engine"
)


type EBPFHandler struct {
	pid    int
	parser engine.Parser
}

func NewEBPF(pid int, parser engine.Parser) *EBPFHandler {
	return &EBPFHandler{pid: pid, parser: parser}
}

func (h *EBPFHandler) Type() string {
	return "ebpf"
}

func (h *EBPFHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	return fmt.Errorf("eBPF interception is natively supported on Linux only. Please run this command on a Linux system or WSL2")
}

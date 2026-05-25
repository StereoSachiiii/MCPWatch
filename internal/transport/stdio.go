package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"mcpwatch/internal/engine"
)

// StdioHandler wraps a child process and intercepts its stdin/stdout pipes.
type StdioHandler struct {
	command string
}

// NewStdio creates a new StdioHandler for the given command string.
func NewStdio(command string) *StdioHandler {
	return &StdioHandler{command: command}
}

// Type returns "stdio".
func (h *StdioHandler) Type() string {
	return "stdio"
}

// Start launches the child process and intercepts all stdin/stdout traffic.
// Messages are forwarded transparently while copies are parsed and sent to the channel.
func (h *StdioHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	if h.command == "" {
		return fmt.Errorf("empty command")
	}

	// Use the OS shell to handle paths with spaces, quoting, etc.
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd", "/C", h.command)
	} else {
		cmd = exec.CommandContext(ctx, "sh", "-c", h.command)
	}

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	// Client → Server (stdin interception)
	go func() {
		reader := io.TeeReader(os.Stdin, stdinPipe)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if msg := engine.ParseJSONRPC(line, "IN", h.Type()); msg != nil {
				messages <- msg
			}
		}
		stdinPipe.Close()
	}()

	// Server → Client (stdout interception)
	reader := io.TeeReader(stdoutPipe, os.Stdout)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if msg := engine.ParseJSONRPC(line, "OUT", h.Type()); msg != nil {
			messages <- msg
		}
	}

	return cmd.Wait()
}

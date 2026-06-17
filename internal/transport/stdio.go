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


type StdioHandler struct {
	command string
	parser  engine.Parser
}


func NewStdio(command string, parser engine.Parser) *StdioHandler {
	return &StdioHandler{command: command, parser: parser}
}


func (h *StdioHandler) Type() string {
	return "stdio"
}



func (h *StdioHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	if h.command == "" {
		return fmt.Errorf("empty command")
	}

	
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

	
	go func() {
		reader := io.TeeReader(os.Stdin, stdinPipe)
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			if msg := h.parser.Parse(line, "IN", h.Type()); msg != nil {
				select {
				case messages <- msg:
				default:
				}
			}
		}
		stdinPipe.Close()
	}()

	
	reader := io.TeeReader(stdoutPipe, os.Stdout)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if msg := h.parser.Parse(line, "OUT", h.Type()); msg != nil {
			select {
			case messages <- msg:
			default:
			}
		}
	}

	return cmd.Wait()
}

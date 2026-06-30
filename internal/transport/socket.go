package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"

	"mcpwatch/internal/engine"
)

type SocketHandler struct {
	localPath  string
	targetPath string
	parser     engine.Parser
}

func NewSocket(localPath, targetPath string, parser engine.Parser) *SocketHandler {
	return &SocketHandler{
		localPath:  localPath,
		targetPath: targetPath,
		parser:     parser,
	}
}

func (h *SocketHandler) Type() string {
	return "socket"
}

func (h *SocketHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	// Clean up local socket path if it already exists
	_ = os.Remove(h.localPath)

	lc := net.ListenConfig{}
	listener, err := lc.Listen(ctx, "unix", h.localPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket: %w", err)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		listener.Close()
		_ = os.Remove(h.localPath)
	}()

	slog.Info("Proxying Unix socket traffic", "local_path", h.localPath, "target_path", h.targetPath)

	for {
		clientConn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				slog.Error("failed to accept socket connection", "error", err)
				continue
			}
		}

		go h.handleConnection(ctx, clientConn, messages)
	}
}

func (h *SocketHandler) handleConnection(ctx context.Context, clientConn net.Conn, messages chan<- *engine.Message) {
	defer clientConn.Close()

	var dialer net.Dialer
	targetConn, err := dialer.DialContext(ctx, "unix", h.targetPath)
	if err != nil {
		slog.Error("failed to connect to target socket", "path", h.targetPath, "error", err)
		return
	}
	defer targetConn.Close()

	clientWriter := &socketInterceptor{
		Writer:    targetConn,
		messages:  messages,
		transType: h.Type(),
		direction: "IN",
		parser:    h.parser,
	}

	targetWriter := &socketInterceptor{
		Writer:    clientConn,
		messages:  messages,
		transType: h.Type(),
		direction: "OUT",
		parser:    h.parser,
	}

	errChan := make(chan error, 2)

	go func() {
		_, err := io.Copy(clientWriter, clientConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(targetWriter, targetConn)
		errChan <- err
	}()

	select {
	case <-ctx.Done():
	case <-errChan:
	}
}

type socketInterceptor struct {
	io.Writer
	messages  chan<- *engine.Message
	transType string
	direction string
	buf       []byte
	parser    engine.Parser
}

func (w *socketInterceptor) Write(p []byte) (n int, err error) {
	n, err = w.Writer.Write(p)
	if n > 0 {
		w.buf = append(w.buf, p[:n]...)
		for {
			idx := bytes.IndexByte(w.buf, '\n')
			if idx == -1 {
				break
			}
			lineBytes := w.buf[:idx]
			w.buf = w.buf[idx+1:]

			lineBytes = bytes.TrimSpace(lineBytes)
			line := string(lineBytes)
			if line == "" {
				continue
			}

			if msg := w.parser.Parse(line, w.direction, w.transType); msg != nil {
				select {
				case w.messages <- msg:
				default:
				}
			}
		}
	}
	return n, err
}

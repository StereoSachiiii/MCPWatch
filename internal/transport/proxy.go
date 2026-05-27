package transport

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"mcpwatch/internal/engine"
)

// ProxyHandler proxies HTTP and SSE traffic to a remote MCP server and intercepts JSON-RPC.
type ProxyHandler struct {
	targetURL string
	localPort string
}

func NewProxy(targetURL, localPort string) *ProxyHandler {
	return &ProxyHandler{
		targetURL: targetURL,
		localPort: localPort,
	}
}

func (h *ProxyHandler) Type() string {
	return "http/sse"
}

func (h *ProxyHandler) Start(ctx context.Context, messages chan<- *engine.Message) error {
	target, err := url.Parse(h.targetURL)
	if err != nil {
		return fmt.Errorf("invalid proxy target URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &interceptingTransport{
		RoundTripper:  http.DefaultTransport,
		messages:      messages,
		transportType: h.Type(),
	}

	server := &http.Server{
		Addr:    ":" + h.localPort,
		Handler: proxy,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("[MCPWatch] Proxying HTTP/SSE traffic from :%s to %s\n", h.localPort, h.targetURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

type interceptingTransport struct {
	http.RoundTripper
	messages      chan<- *engine.Message
	transportType string
}

func (t *interceptingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Intercept Request (Client -> Server)
	if req.Body != nil {
		bodyBytes, _ := io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		
		line := string(bodyBytes)
		if msg := engine.ParseJSONRPC(line, "IN", t.transportType); msg != nil {
			select {
			case t.messages <- msg:
			default:
			}
		}
	}

	res, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return res, err
	}

	// Intercept Response (Server -> Client)
	if res.Body != nil {
		contentType := res.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "text/event-stream") {
			// SSE Stream
			res.Body = &sseInterceptor{
				ReadCloser: res.Body,
				messages:   t.messages,
				transType:  t.transportType,
			}
		} else {
			// Standard HTTP Response (e.g. POST replies)
			bodyBytes, _ := io.ReadAll(res.Body)
			res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
			line := string(bodyBytes)
			if msg := engine.ParseJSONRPC(line, "OUT", t.transportType); msg != nil {
				select {
				case t.messages <- msg:
				default:
				}
			}
		}
	}

	return res, err
}

type sseInterceptor struct {
	io.ReadCloser
	messages  chan<- *engine.Message
	transType string
	buf       []byte
}

func (s *sseInterceptor) Read(p []byte) (n int, err error) {
	n, err = s.ReadCloser.Read(p)
	if n > 0 {
		if len(s.buf)+n > 10*1024*1024 {
			return 0, fmt.Errorf("sse stream buffer limit exceeded (10MB)")
		}
		s.buf = append(s.buf, p[:n]...)
		for {
			idx := bytes.IndexByte(s.buf, '\n')
			if idx == -1 {
				break
			}
			line := string(s.buf[:idx])
			s.buf = s.buf[idx+1:]
			
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "data:") {
				payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				if payload != "" && (strings.HasPrefix(payload, "{") || strings.HasPrefix(payload, "[")) {
					if msg := engine.ParseJSONRPC(payload, "OUT", s.transType); msg != nil {
						select {
						case s.messages <- msg:
						default:
						}
					}
				}
			}
		}
	}
	return n, err
}

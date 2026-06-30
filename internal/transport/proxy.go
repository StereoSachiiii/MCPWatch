package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"mcpwatch/internal/engine"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)


type ProxyHandler struct {
	targetURL string
	localPort string
	parser    engine.Parser
}

func NewProxy(targetURL, localPort string, parser engine.Parser) *ProxyHandler {
	return &ProxyHandler{
		targetURL: targetURL,
		localPort: localPort,
		parser:    parser,
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
		parser:        h.parser,
	}

	server := &http.Server{
		Addr:    ":" + h.localPort,
		Handler: proxy,
	}

	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	slog.Info("Proxying HTTP/SSE traffic", "local_port", h.localPort, "target_url", h.targetURL)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

type interceptingTransport struct {
	http.RoundTripper
	messages      chan<- *engine.Message
	transportType string
	parser        engine.Parser
}

func (t *interceptingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var parentCtx context.Context = req.Context()
	var bodyBytes []byte

	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		var paramsObj struct {
			Meta struct {
				Traceparent string `json:"traceparent"`
			} `json:"meta"`
			MetaAlt struct {
				Traceparent string `json:"traceparent"`
			} `json:"_meta"`
		}
		var jsonRPCReq struct {
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(bodyBytes, &jsonRPCReq); err == nil && len(jsonRPCReq.Params) > 0 {
			_ = json.Unmarshal(jsonRPCReq.Params, &paramsObj)
		}

		tp := paramsObj.Meta.Traceparent
		if tp == "" {
			tp = paramsObj.MetaAlt.Traceparent
		}
		if tp != "" {
			carrier := propagation.MapCarrier{"traceparent": tp}
			propagator := propagation.TraceContext{}
			parentCtx = propagator.Extract(parentCtx, carrier)
		}
	}

	tracer := otel.Tracer("mcpwatch")
	var span trace.Span
	if tracer != nil {
		parentCtx, span = tracer.Start(parentCtx, "mcp.proxy.forward", trace.WithSpanKind(trace.SpanKindClient))
		defer span.End()
	}

	propagator := otel.GetTextMapPropagator()
	if propagator != nil {
		propagator.Inject(parentCtx, propagation.HeaderCarrier(req.Header))
	}

	if req.Body != nil {
		req.Body = &bodyInterceptor{
			ReadCloser: io.NopCloser(bytes.NewBuffer(bodyBytes)),
			messages:   t.messages,
			transType:  t.transportType,
			direction:  "IN",
			parser:     t.parser,
		}
	}

	res, err := t.RoundTripper.RoundTrip(req)
	if err != nil {
		return res, err
	}

	if res.Body != nil {
		contentType := res.Header.Get("Content-Type")
		if strings.HasPrefix(contentType, "text/event-stream") ||
			strings.HasPrefix(contentType, "application/x-ndjson") ||
			strings.HasPrefix(contentType, "application/json-seq") ||
			strings.Contains(contentType, "ndjson") {

			res.Body = &streamInterceptor{
				ReadCloser: res.Body,
				messages:   t.messages,
				transType:  t.transportType,
				parser:     t.parser,
			}
		} else {
			res.Body = &bodyInterceptor{
				ReadCloser: res.Body,
				messages:   t.messages,
				transType:  t.transportType,
				direction:  "OUT",
				parser:     t.parser,
			}
		}
	}

	return res, err
}

type bodyInterceptor struct {
	io.ReadCloser
	messages  chan<- *engine.Message
	transType string
	direction string
	buf       []byte
	parser    engine.Parser
}

func (s *bodyInterceptor) Read(p []byte) (n int, err error) {
	n, err = s.ReadCloser.Read(p)
	if n > 0 {
		if len(s.buf)+n > 10*1024*1024 {
			return 0, fmt.Errorf("stream buffer limit exceeded (10MB)")
		}
		s.buf = append(s.buf, p[:n]...)
		for {
			idx := bytes.IndexByte(s.buf, '\n')
			if idx == -1 {
				break
			}
			lineBytes := s.buf[:idx]
			s.buf = s.buf[idx+1:]

			lineBytes = bytes.TrimSpace(lineBytes)
			lineBytes = bytes.TrimPrefix(lineBytes, []byte("\x1e"))
			line := string(lineBytes)

			if line == "" {
				continue
			}

			if msg := s.parser.Parse(line, s.direction, s.transType); msg != nil {
				select {
				case s.messages <- msg:
				default:
				}
			}
		}
	}
	if err == io.EOF {
		if len(s.buf) > 0 {
			lineBytes := bytes.TrimSpace(s.buf)
			lineBytes = bytes.TrimPrefix(lineBytes, []byte("\x1e"))
			line := string(lineBytes)
			s.buf = nil
			if line != "" {
				if msg := s.parser.Parse(line, s.direction, s.transType); msg != nil {
					select {
					case s.messages <- msg:
					default:
					}
				}
			}
		}
	}
	return n, err
}

type streamInterceptor struct {
	io.ReadCloser
	messages  chan<- *engine.Message
	transType string
	buf       []byte
	parser    engine.Parser
}

func (s *streamInterceptor) Read(p []byte) (n int, err error) {
	n, err = s.ReadCloser.Read(p)
	if n > 0 {
		if len(s.buf)+n > 10*1024*1024 {
			return 0, fmt.Errorf("stream buffer limit exceeded (10MB)")
		}
		s.buf = append(s.buf, p[:n]...)
		for {
			idx := bytes.IndexByte(s.buf, '\n')
			if idx == -1 {
				break
			}
			lineBytes := s.buf[:idx]
			s.buf = s.buf[idx+1:]

			lineBytes = bytes.TrimSpace(lineBytes)
			lineBytes = bytes.TrimPrefix(lineBytes, []byte("\x1e"))
			line := string(lineBytes)

			if line == "" {
				continue
			}

			var payload string
			if strings.HasPrefix(line, "data:") {
				payload = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			} else {
				payload = line
			}

			if payload != "" && (strings.HasPrefix(payload, "{") || strings.HasPrefix(payload, "[")) {
				if msg := s.parser.Parse(payload, "OUT", s.transType); msg != nil {
					select {
					case s.messages <- msg:
					default:
					}
				}
			}
		}
	}
	if err == io.EOF {
		if len(s.buf) > 0 {
			lineBytes := bytes.TrimSpace(s.buf)
			lineBytes = bytes.TrimPrefix(lineBytes, []byte("\x1e"))
			line := string(lineBytes)
			s.buf = nil
			if line != "" {
				var payload string
				if strings.HasPrefix(line, "data:") {
					payload = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
				} else {
					payload = line
				}
				if payload != "" && (strings.HasPrefix(payload, "{") || strings.HasPrefix(payload, "[")) {
					if msg := s.parser.Parse(payload, "OUT", s.transType); msg != nil {
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

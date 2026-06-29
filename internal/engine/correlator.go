package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const NumSlots = 4096

type PendingSlot struct {
	JSONRPCID string
	SentAt    time.Time
	Method    string
	Span      trace.Span
}

type Correlator struct {
	mu    sync.Mutex
	slots [NumSlots]PendingSlot
}

func NewCorrelator() *Correlator {
	return &Correlator{}
}

func hashString(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

func (c *Correlator) Process(msg *Message) {
	if msg.JSONRPCID == "" {
		return
	}

	idx := hashString(msg.JSONRPCID) % NumSlots

	c.mu.Lock()
	defer c.mu.Unlock()

	if msg.MsgType == MsgTypeRequest {
		parentCtx := extractParentContext(msg)
		var span trace.Span
		tracer := otel.Tracer("mcpwatch")
		if tracer != nil {
			_, span = tracer.Start(parentCtx, "mcp.request."+msg.Method,
				trace.WithAttributes(
					attribute.String("mcp.method", msg.Method),
					attribute.String("mcp.transport", msg.Transport),
					attribute.String("mcp.direction", msg.Direction),
					attribute.String("mcp.jsonrpc_id", msg.JSONRPCID),
					attribute.Int64("mcp.request_bytes", msg.SizeBytes),
				),
			)
		}

		c.slots[idx] = PendingSlot{
			JSONRPCID: msg.JSONRPCID,
			SentAt:    msg.Timestamp,
			Method:    msg.Method,
			Span:      span,
		}
	} else if msg.MsgType == MsgTypeResponse {
		slot := c.slots[idx]

		if slot.JSONRPCID == msg.JSONRPCID {
			msg.LatencyMS = time.Since(slot.SentAt).Milliseconds()
			if msg.Method == "" {
				msg.Method = slot.Method
			}

			if slot.Span != nil {
				if msg.ErrorCode != "" || (msg.ErrorData != "" && msg.ErrorData != "null") {
					slot.Span.SetStatus(codes.Error, fmt.Sprintf("Error Code: %s", msg.ErrorCode))
					slot.Span.SetAttributes(
						attribute.String("mcp.error_code", msg.ErrorCode),
						attribute.String("mcp.error_data", msg.ErrorData),
					)
				} else {
					slot.Span.SetStatus(codes.Ok, "success")
				}
				slot.Span.SetAttributes(
					attribute.Int64("mcp.response_bytes", msg.SizeBytes),
					attribute.Int64("mcp.latency_ms", msg.LatencyMS),
				)
				slot.Span.End()
			}

			c.slots[idx] = PendingSlot{}
		}
	}
}

type MetaCarrier map[string]string

func (m MetaCarrier) Get(key string) string { return m[key] }
func (m MetaCarrier) Set(key, value string) { m[key] = value }
func (m MetaCarrier) Keys() []string       { return []string{"traceparent"} }

func extractParentContext(msg *Message) context.Context {
	if msg.Params == "" {
		return context.Background()
	}
	var paramsObj struct {
		Meta struct {
			Traceparent string `json:"traceparent"`
		} `json:"meta"`
		MetaAlt struct {
			Traceparent string `json:"traceparent"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal([]byte(msg.Params), &paramsObj); err == nil {
		tp := paramsObj.Meta.Traceparent
		if tp == "" {
			tp = paramsObj.MetaAlt.Traceparent
		}
		if tp != "" {
			carrier := MetaCarrier{"traceparent": tp}
			propagator := propagation.TraceContext{}
			return propagator.Extract(context.Background(), carrier)
		}
	}
	return context.Background()
}

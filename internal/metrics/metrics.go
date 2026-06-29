package metrics

import (
	"mcpwatch/internal/engine"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	MessagesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_messages_total",
			Help: "Total number of intercepted MCP messages.",
		},
		[]string{"transport", "direction", "msg_type"},
	)

	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_requests_total",
			Help: "Total number of MCP requests.",
		},
		[]string{"method", "status"},
	)

	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_request_duration_seconds",
			Help:    "Latency of MCP requests in seconds.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
		},
		[]string{"method"},
	)

	BytesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_bytes_total",
			Help: "Total size of intercepted payloads in bytes.",
		},
		[]string{"transport", "direction"},
	)

	TokensTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_tokens_estimated_total",
			Help: "Estimated number of tokens processed.",
		},
		[]string{"transport"},
	)
)

// RecordMetrics updates Prometheus vectors with message info
func RecordMetrics(msg *engine.Message) {
	if msg == nil {
		return
	}

	// Record general message count
	MessagesTotal.WithLabelValues(
		msg.Transport,
		msg.Direction,
		string(msg.MsgType),
	).Inc()

	// Record payload sizes in bytes
	BytesTotal.WithLabelValues(
		msg.Transport,
		msg.Direction,
	).Add(float64(msg.SizeBytes))

	// Record token usage estimate
	if msg.TokenEstimate > 0 {
		TokensTotal.WithLabelValues(
			msg.Transport,
		).Add(float64(msg.TokenEstimate))
	}

	// Record requests volume
	if msg.MsgType == engine.MsgTypeRequest {
		status := "success"
		if msg.ErrorCode != "" || (msg.ErrorData != "" && msg.ErrorData != "null") {
			status = "error"
		}
		RequestsTotal.WithLabelValues(
			msg.Method,
			status,
		).Inc()
	}

	// Record responses volume and latency distributions
	if msg.MsgType == engine.MsgTypeResponse {
		status := "success"
		if msg.ErrorCode != "" || (msg.ErrorData != "" && msg.ErrorData != "null") {
			status = "error"
		}
		RequestsTotal.WithLabelValues(
			msg.Method,
			status,
		).Inc()

		if msg.LatencyMS > 0 {
			durationSec := float64(msg.LatencyMS) / 1000.0
			RequestDuration.WithLabelValues(
				msg.Method,
			).Observe(durationSec)
		}
	}
}

package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"mcpwatch/internal/storage"
)

type Alerter struct {
	store              *storage.Store
	webhookURL         string
	errorRateThreshold float64
	latencyThreshold   int64
	window             time.Duration
	lastAlerted        time.Time
}

func New(store *storage.Store, webhook string, errorRate float64, latency int64, windowSeconds int) *Alerter {
	return &Alerter{
		store:              store,
		webhookURL:         webhook,
		errorRateThreshold: errorRate,
		latencyThreshold:   latency,
		window:             time.Duration(windowSeconds) * time.Second,
	}
}

// Start spawns the alerting ticker and runs until context is closed.
func (a *Alerter) Start(ctx context.Context) {
	if a.webhookURL == "" {
		return
	}
	slog.Info("Alerting engine initialized",
		"webhook", a.webhookURL,
		"error_threshold_pct", a.errorRateThreshold,
		"latency_threshold_ms", a.latencyThreshold,
		"window_sec", a.window.Seconds(),
	)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.check(ctx)
		}
	}
}

func (a *Alerter) check(ctx context.Context) {
	messages, err := a.store.QueryAll()
	if err != nil || len(messages) == 0 {
		return
	}

	cutoff := time.Now().Add(-a.window)
	var totalMsgs int
	var errorMsgs int
	var totalLatency int64
	var latencyCount int64

	for _, m := range messages {
		if m.Timestamp.After(cutoff) {
			// Count total messages of type request / response (we exclude raw stdin/stdout streams)
			if m.MsgType == "request" || m.MsgType == "response" {
				totalMsgs++
				if m.ErrorCode != "" || (m.ErrorData != "" && m.ErrorData != "null") {
					errorMsgs++
				}
			}
			if m.MsgType == "response" && m.LatencyMS > 0 {
				totalLatency += m.LatencyMS
				latencyCount++
			}
		}
	}

	if totalMsgs == 0 {
		return
	}

	errorRate := (float64(errorMsgs) / float64(totalMsgs)) * 100
	var avgLatency int64
	if latencyCount > 0 {
		avgLatency = totalLatency / latencyCount
	}

	shouldAlert := false
	var reason string

	if a.errorRateThreshold > 0 && errorRate > a.errorRateThreshold {
		shouldAlert = true
		reason = fmt.Sprintf("Error rate is %.1f%% (threshold: %.1f%%)", errorRate, a.errorRateThreshold)
	}

	if !shouldAlert && a.latencyThreshold > 0 && avgLatency > a.latencyThreshold {
		shouldAlert = true
		reason = fmt.Sprintf("Average request latency is %dms (threshold: %dms)", avgLatency, a.latencyThreshold)
	}

	if shouldAlert {
		if time.Since(a.lastAlerted) > a.window {
			a.fireAlert(ctx, reason, errorRate, avgLatency)
			a.lastAlerted = time.Now()
		}
	}
}

func (a *Alerter) fireAlert(ctx context.Context, reason string, errorRate float64, avgLatency int64) {
	slog.Warn("Observability threshold breached! Dispatching Webhook alert...", "reason", reason)

	payload := map[string]interface{}{
		"text": fmt.Sprintf("⚠️ *MCPWatch Alert Triggered* ⚠️\n*Reason*: %s\n*Current Error Rate*: %.1f%%\n*Current Avg Latency*: %dms\n*Time*: %s",
			reason, errorRate, avgLatency, time.Now().Format(time.RFC3339)),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal webhook alert payload", "error", err)
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.webhookURL, bytes.NewBuffer(body))
	if err != nil {
		slog.Error("failed to create webhook request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("failed to dispatch webhook alert", "error", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		slog.Error("alert webhook returned failure status", "status", resp.Status)
	} else {
		slog.Info("Alert webhook successfully dispatched!")
	}
}

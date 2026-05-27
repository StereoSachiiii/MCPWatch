package engine

import (
	"sync"
	"time"
)

type pendingRequest struct {
	sentAt time.Time
	method string
}

// Correlator matches JSON-RPC requests with their responses to compute latency.
type Correlator struct {
	mu      sync.Mutex
	pending map[string]pendingRequest
}

// NewCorrelator creates a new Correlator instance with a background cleanup goroutine.
func NewCorrelator() *Correlator {
	c := &Correlator{
		pending: make(map[string]pendingRequest),
	}
	go c.cleanupLoop()
	return c
}

// Process enriches a message with correlation data.
// For requests: records the timestamp keyed by JSON-RPC ID.
// For responses: matches the ID, computes latency, carries the method forward.
func (c *Correlator) Process(msg *Message) {
	if msg.JSONRPCID == "" {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	switch msg.MsgType {
	case MsgTypeRequest:
		c.pending[msg.JSONRPCID] = pendingRequest{
			sentAt: msg.Timestamp,
			method: msg.Method,
		}
	case MsgTypeResponse:
		if req, ok := c.pending[msg.JSONRPCID]; ok {
			msg.LatencyMS = time.Since(req.sentAt).Milliseconds()
			if msg.Method == "" {
				msg.Method = req.method
			}
			delete(c.pending, msg.JSONRPCID)
		}
	}
}

// PendingCount returns the number of unmatched requests.
func (c *Correlator) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pending)
}

func (c *Correlator) cleanupLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for id, req := range c.pending {
			if now.Sub(req.sentAt) > 30*time.Second {
				delete(c.pending, id)
			}
		}
		c.mu.Unlock()
	}
}

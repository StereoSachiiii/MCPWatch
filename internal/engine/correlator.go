package engine

import (
	"sync"
	"time"
)

const NumSlots = 4096

// PendingSlot holds the data for an in-flight request.
type PendingSlot struct {
	JSONRPCID string
	SentAt    time.Time
	Method    string
}

// Correlator uses a zero-allocation, lossy hash table to match requests with responses.
// This provides bounded memory (no heap growth) and O(1) latency tracking.
// Inspired by VictoriaMetrics fastcache chunks.
type Correlator struct {
	mu    sync.Mutex
	slots [NumSlots]PendingSlot
}

// NewCorrelator creates a new zero-allocation Correlator.
func NewCorrelator() *Correlator {
	return &Correlator{}
}

// hash string to uint32 for O(1) slot mapping
func hashString(s string) uint32 {
	var h uint32
	for i := 0; i < len(s); i++ {
		h = h*31 + uint32(s[i])
	}
	return h
}

// Process enriches a message with correlation data.
func (c *Correlator) Process(msg *Message) {
	if msg.JSONRPCID == "" {
		return
	}

	// Map the JSON-RPC ID to a fixed slot using a hash.
	// In the rare case of a collision within the active window, the older request is overwritten (lossy).
	idx := hashString(msg.JSONRPCID) % NumSlots

	c.mu.Lock()
	defer c.mu.Unlock()

	if msg.MsgType == MsgTypeRequest {
		c.slots[idx] = PendingSlot{
			JSONRPCID: msg.JSONRPCID,
			SentAt:    msg.Timestamp,
			Method:    msg.Method,
		}
	} else if msg.MsgType == MsgTypeResponse {
		slot := c.slots[idx]
		// Verify this slot actually belongs to our response (handles collisions)
		if slot.JSONRPCID == msg.JSONRPCID {
			msg.LatencyMS = time.Since(slot.SentAt).Milliseconds()
			if msg.Method == "" {
				msg.Method = slot.Method
			}
			// Clear the slot
			c.slots[idx] = PendingSlot{}
		}
	}
}

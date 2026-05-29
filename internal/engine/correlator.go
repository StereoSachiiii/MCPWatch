package engine

import (
	"sync"
	"time"
)

const NumSlots = 4096


type PendingSlot struct {
	JSONRPCID string
	SentAt    time.Time
	Method    string
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
		c.slots[idx] = PendingSlot{
			JSONRPCID: msg.JSONRPCID,
			SentAt:    msg.Timestamp,
			Method:    msg.Method,
		}
	} else if msg.MsgType == MsgTypeResponse {
		slot := c.slots[idx]

		if slot.JSONRPCID == msg.JSONRPCID {
			msg.LatencyMS = time.Since(slot.SentAt).Milliseconds()
			if msg.Method == "" {
				msg.Method = slot.Method
			}

			c.slots[idx] = PendingSlot{}
		}
	}
}

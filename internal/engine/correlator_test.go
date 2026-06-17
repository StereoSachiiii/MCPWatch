package engine

import (
	"testing"
	"time"
)

func TestCorrelator_Process(t *testing.T) {
	c := NewCorrelator()

	// 1. Basic request-response correlation
	reqTime := time.Now().Add(-50 * time.Millisecond)
	req := &Message{
		JSONRPCID: "req-1",
		MsgType:   MsgTypeRequest,
		Method:    "ping",
		Timestamp: reqTime,
	}

	resp := &Message{
		JSONRPCID: "req-1",
		MsgType:   MsgTypeResponse,
		Timestamp: time.Now(),
	}

	c.Process(req)
	c.Process(resp)

	if resp.LatencyMS < 40 || resp.LatencyMS > 200 {
		t.Errorf("Expected LatencyMS around 50ms, got %dms", resp.LatencyMS)
	}

	if resp.Method != "ping" {
		t.Errorf("Expected Method to be propagated as 'ping', got %q", resp.Method)
	}

	// Verify slot is cleared (subsequent response to same ID shouldn't match)
	resp2 := &Message{
		JSONRPCID: "req-1",
		MsgType:   MsgTypeResponse,
		Timestamp: time.Now(),
	}
	c.Process(resp2)
	if resp2.LatencyMS != 0 {
		t.Errorf("Expected secondary response LatencyMS to be 0, got %d", resp2.LatencyMS)
	}

	// 2. Ignore notification
	notif := &Message{
		JSONRPCID: "",
		MsgType:   MsgTypeNotification,
		Method:    "someEvent",
		Timestamp: time.Now(),
	}
	c.Process(notif)
	// Just verify processing a notification without ID does not fail
	if notif.LatencyMS != 0 {
		t.Errorf("Expected notification LatencyMS to be 0")
	}

	// 3. Response without prior request
	respOrphan := &Message{
		JSONRPCID: "orphan-1",
		MsgType:   MsgTypeResponse,
		Timestamp: time.Now(),
	}
	c.Process(respOrphan)
	if respOrphan.LatencyMS != 0 {
		t.Errorf("Expected orphan response LatencyMS to be 0, got %d", respOrphan.LatencyMS)
	}

	// 4. Hash collision / Slot overwrite behavior
	// We want to force a collision by using two requests that map to the same slot index.
	// Since the slot index is hashString(id) % NumSlots, we can construct or find two IDs that collide.
	// Let's compute their hashes in-test.
	var id1, id2 string
	// Find two strings that collide under hashString % NumSlots
	// Since NumSlots is 4096, it's very easy.
	found := false
	for i := 0; i < 10000; i++ {
		for j := i + 1; j < 10000; j++ {
			s1 := "id-" + string(rune(i))
			s2 := "id-" + string(rune(j))
			if hashString(s1)%NumSlots == hashString(s2)%NumSlots {
				id1 = s1
				id2 = s2
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		t.Fatal("Could not find colliding IDs for hash collision test")
	}

	reqCollision1 := &Message{
		JSONRPCID: id1,
		MsgType:   MsgTypeRequest,
		Method:    "method1",
		Timestamp: time.Now().Add(-100 * time.Millisecond),
	}
	reqCollision2 := &Message{
		JSONRPCID: id2,
		MsgType:   MsgTypeRequest,
		Method:    "method2",
		Timestamp: time.Now().Add(-10 * time.Millisecond),
	}

	// Process first request
	c.Process(reqCollision1)
	// Process second request which collides and overwrites the slot
	c.Process(reqCollision2)

	// A response for request 1 should now NOT be matched, since it was overwritten
	respCollision1 := &Message{
		JSONRPCID: id1,
		MsgType:   MsgTypeResponse,
		Timestamp: time.Now(),
	}
	c.Process(respCollision1)
	if respCollision1.LatencyMS != 0 {
		t.Errorf("Expected overwritten slot response LatencyMS to be 0, got %d", respCollision1.LatencyMS)
	}

	// A response for request 2 should match and have the correct latency/method
	respCollision2 := &Message{
		JSONRPCID: id2,
		MsgType:   MsgTypeResponse,
		Timestamp: time.Now(),
	}
	c.Process(respCollision2)
	if respCollision2.LatencyMS < 5 || respCollision2.LatencyMS > 50 {
		t.Errorf("Expected request 2 latency to be around 10ms, got %dms", respCollision2.LatencyMS)
	}
	if respCollision2.Method != "method2" {
		t.Errorf("Expected request 2 method to be 'method2', got %q", respCollision2.Method)
	}
}

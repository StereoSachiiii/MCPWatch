package storage

import (
	"path/filepath"
	"testing"
	"time"

	"mcpwatch/internal/engine"
)

func TestStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "badger_test")

	// 1. Initialize store
	store, err := New(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize store: %v", err)
	}
	defer store.Close()

	// 2. Validate clean initial stats
	stats, err := store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats.TotalMessages != 0 {
		t.Errorf("Expected 0 initial messages, got %d", stats.TotalMessages)
	}

	// 3. Insert a request message
	msgReq := &engine.Message{
		Timestamp: time.Now(),
		Transport: "stdio",
		Direction: "IN",
		MsgType:   engine.MsgTypeRequest,
		Method:    "initialize",
		SizeBytes: 100,
		TokenEstimate: 25,
	}
	if err := store.Insert(msgReq); err != nil {
		t.Fatalf("Failed to insert request message: %v", err)
	}

	// Wait for async flush to run
	time.Sleep(100 * time.Millisecond)

	// Verify stats updated
	stats, err = store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats.TotalMessages != 1 || stats.TotalRequests != 1 {
		t.Errorf("Expected 1 message/request, got %d messages, %d requests", stats.TotalMessages, stats.TotalRequests)
	}
	if stats.TotalBytes != 100 || stats.TotalTokens != 25 {
		t.Errorf("Expected 100 bytes and 25 tokens, got %d bytes and %d tokens", stats.TotalBytes, stats.TotalTokens)
	}

	// 4. Insert a response message (with latency)
	msgResp := &engine.Message{
		Timestamp: time.Now(),
		Transport: "stdio",
		Direction: "OUT",
		MsgType:   engine.MsgTypeResponse,
		Method:    "initialize",
		SizeBytes: 200,
		TokenEstimate: 50,
		LatencyMS: 50,
	}
	if err := store.Insert(msgResp); err != nil {
		t.Fatalf("Failed to insert response message: %v", err)
	}

	// Wait for async flush
	time.Sleep(100 * time.Millisecond)

	stats, err = store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}
	if stats.TotalMessages != 2 {
		t.Errorf("Expected 2 messages, got %d", stats.TotalMessages)
	}
	// Avg latency of 2 messages (1st has 0 latency, 2nd has 50ms) -> 50 / 2 = 25ms
	if stats.AvgLatencyMS != 25.0 {
		t.Errorf("Expected AvgLatencyMS to be 25.0, got %f", stats.AvgLatencyMS)
	}

	// 5. Query messages
	msgs, err := store.QueryRecent(10)
	if err != nil {
		t.Fatalf("QueryRecent failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("Expected 2 queried messages, got %d", len(msgs))
	}
	// Reverse order check (QueryRecent returns in reverse chronological order)
	if msgs[0].MsgType != engine.MsgTypeResponse {
		t.Errorf("Expected latest message to be response, got %s", msgs[0].MsgType)
	}

	// QueryAll (chronological order)
	allMsgs, err := store.QueryAll()
	if err != nil {
		t.Fatalf("QueryAll failed: %v", err)
	}
	if len(allMsgs) != 2 {
		t.Errorf("Expected 2 messages in QueryAll, got %d", len(allMsgs))
	}
	if allMsgs[0].MsgType != engine.MsgTypeRequest {
		t.Errorf("Expected first message in QueryAll to be request, got %s", allMsgs[0].MsgType)
	}

	// 6. Clear database
	if err := store.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	stats, err = store.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats after clear: %v", err)
	}
	if stats.TotalMessages != 0 || stats.TotalBytes != 0 {
		t.Errorf("Expected stats to reset, got %+v", stats)
	}

	msgs, err = store.QueryRecent(10)
	if err != nil {
		t.Fatalf("QueryRecent failed after clear: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("Expected 0 queried messages after clear, got %d", len(msgs))
	}
}

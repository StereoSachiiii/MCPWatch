package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestHub(t *testing.T) {
	hub := NewHub()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}

		client := hub.Register(conn)
		ctx := r.Context()
		for {
			if _, _, err := conn.Read(ctx); err != nil {
				hub.Unregister(client)
				break
			}
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	wsURL := strings.Replace(server.URL, "http", "ws", 1)
	clientConn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial websocket: %v", err)
	}
	defer clientConn.Close(websocket.StatusNormalClosure, "")

	// Wait briefly for registration to complete
	time.Sleep(50 * time.Millisecond)

	count := hub.ClientCount()
	if count != 1 {
		t.Errorf("expected 1 registered client, got %d", count)
	}

	// Broadcast
	testMsg := map[string]string{"foo": "bar"}
	hub.Broadcast(testMsg)

	// Read on client side
	msgType, p, err := clientConn.Read(ctx)
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	if msgType != websocket.MessageText {
		t.Errorf("expected MessageText type, got %v", msgType)
	}

	expectedStr := `{"foo":"bar"}`
	if string(p) != expectedStr {
		t.Errorf("expected message body %q, got %q", expectedStr, string(p))
	}

	// Close client connection and verify unregistration
	clientConn.Close(websocket.StatusNormalClosure, "")

	// Allow loop to detect closure and unregister
	time.Sleep(100 * time.Millisecond)

	count = hub.ClientCount()
	if count != 0 {
		t.Errorf("expected 0 clients after close, got %d", count)
	}
}

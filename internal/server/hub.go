package server

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Client wraps a WebSocket connection with a non-blocking send channel.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close()
	}()
	for msg := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

// Hub manages WebSocket connections and broadcasts messages to all clients without blocking.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
}

// NewHub creates a new WebSocket hub.
func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
	}
}

// Register adds a WebSocket connection to the hub and starts its pump.
func (h *Hub) Register(conn *websocket.Conn) *Client {
	client := &Client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	go client.writePump()
	return client
}

// Unregister removes a WebSocket connection from the hub.
func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}

// Broadcast sends a JSON-encoded message to all connected WebSocket clients.
// If a client's send buffer is full, it is disconnected to prevent blocking the proxy.
func (h *Hub) Broadcast(data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("[MCPWatch] broadcast marshal error: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- payload:
		default:
			// Client is too slow to read messages. Forcibly disconnect to avoid backpressure.
			go h.Unregister(client)
		}
	}
}

// ClientCount returns the number of connected WebSocket clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

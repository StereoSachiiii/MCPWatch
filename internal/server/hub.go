package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"nhooyr.io/websocket"
)


type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}


func (c *Client) writePump() {
	defer func() {
		c.hub.Unregister(c)
		c.conn.Close(websocket.StatusNormalClosure, "")
	}()
	for msg := range c.send {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.conn.Write(ctx, websocket.MessageText, msg); err != nil {
			cancel()
			return
		}
		cancel()
	}
}


type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool
}


func NewHub() *Hub {
	return &Hub{
		clients: make(map[*Client]bool),
	}
}


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


func (h *Hub) Unregister(client *Client) {
	h.mu.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
	}
	h.mu.Unlock()
}



func (h *Hub) Broadcast(data interface{}) {
	payload, err := json.Marshal(data)
	if err != nil {
		slog.Error("broadcast marshal error", "error", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- payload:
		default:

			go h.Unregister(client)
		}
	}
}


func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

package server

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)


type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}


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
		log.Printf("[MCPWatch] broadcast marshal error: %v", err)
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

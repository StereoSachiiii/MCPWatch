package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"net/url"

	"mcpwatch/internal/storage"

	"github.com/gorilla/websocket"
)

// Server provides the HTTP API, WebSocket endpoint, and static file serving.
type Server struct {
	store    *storage.Store
	hub      *Hub
	upgrader websocket.Upgrader
	webFS    fs.FS
}

// New creates a new Server instance.
func New(store *storage.Store, hub *Hub, webFS fs.FS) *Server {
	return &Server{
		store: store,
		hub:   hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				host := u.Hostname()
				return host == "localhost" || host == "127.0.0.1" || u.Host == r.Host
			},
		},
		webFS: webFS,
	}
}

// Start begins serving on the given port. Blocks until error.
func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	// WebSocket endpoint
	mux.HandleFunc("/ws", s.handleWebSocket)

	// REST API
	mux.HandleFunc("/api/interactions", s.handleInteractions)
	mux.HandleFunc("/api/stats", s.handleStats)

	// Static files (dashboard)
	mux.Handle("/", http.FileServer(http.FS(s.webFS)))

	addr := ":" + port
	log.Printf("[MCPWatch] Dashboard: http://localhost:%s", port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[MCPWatch] websocket upgrade error: %v", err)
		return
	}

	client := s.hub.Register(conn)

	// Read loop keeps connection alive; detects client disconnect
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			s.hub.Unregister(client)
			break
		}
	}
}

func (s *Server) handleInteractions(w http.ResponseWriter, r *http.Request) {
	messages, err := s.store.QueryRecent(100)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

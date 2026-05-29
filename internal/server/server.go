package server

import (
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"mcpwatch/internal/storage"

	"nhooyr.io/websocket"
)


type Server struct {
	store    *storage.Store
	hub      *Hub
	webFS    fs.FS
}


func New(store *storage.Store, hub *Hub, webFS fs.FS) *Server {
	return &Server{
		store: store,
		hub:   hub,
		webFS: webFS,
	}
}


func (s *Server) Start(port string) error {
	mux := http.NewServeMux()

	
	mux.HandleFunc("/ws", s.handleWebSocket)

	
	mux.HandleFunc("/api/interactions", s.handleInteractions)
	mux.HandleFunc("/api/stats", s.handleStats)

	
	mux.Handle("/", http.FileServer(http.FS(s.webFS)))

	addr := ":" + port
	log.Printf("[MCPWatch] Dashboard: http://localhost:%s", port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1"},
	})
	if err != nil {
		log.Printf("[MCPWatch] websocket accept error: %v", err)
		return
	}

	client := s.hub.Register(conn)
	ctx := r.Context()

	for {
		if _, _, err := conn.Read(ctx); err != nil {
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

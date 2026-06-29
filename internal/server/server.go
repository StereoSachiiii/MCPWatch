package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"mcpwatch/internal/storage"
	"mcpwatch/internal/utils"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"nhooyr.io/websocket"
)

type Server struct {
	store        *storage.Store
	hub          *Hub
	webFS        fs.FS
	authUsername string
	authPassword string
}

func New(store *storage.Store, hub *Hub, webFS fs.FS) *Server {
	return &Server{
		store: store,
		hub:   hub,
		webFS: webFS,
	}
}

func (s *Server) SetAuth(username, password string) {
	s.authUsername = username
	s.authPassword = password
}

func (s *Server) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.authUsername == "" || s.authPassword == "" {
			next(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.authUsername || pass != s.authPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="MCPWatch Dashboard"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized\n"))
			return
		}
		next(w, r)
	}
}

func (s *Server) basicAuthHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.authUsername == "" || s.authPassword == "" {
			next.ServeHTTP(w, r)
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != s.authUsername || pass != s.authPassword {
			w.Header().Set("WWW-Authenticate", `Basic realm="MCPWatch Dashboard"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "ok",
		"timestamp":         time.Now().Format(time.RFC3339),
		"active_goroutines": utils.GetActiveGoroutineCount(),
	})
}

func (s *Server) setupMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ws", s.basicAuth(s.handleWebSocket))

	mux.HandleFunc("/api/interactions", s.basicAuth(s.handleInteractions))
	mux.HandleFunc("/api/stats", s.basicAuth(s.handleStats))
	mux.HandleFunc("/api/export/json", s.basicAuth(s.handleExportJSON))
	mux.HandleFunc("/api/export/csv", s.basicAuth(s.handleExportCSV))
	mux.HandleFunc("/api/clear", s.basicAuth(s.handleClear))
	mux.Handle("/metrics", promhttp.Handler())

	mux.Handle("/", s.basicAuthHandler(http.FileServer(http.FS(s.webFS))))
	return mux
}

func (s *Server) Start(port string) error {
	mux := s.setupMux()
	addr := ":" + port
	slog.Info("Dashboard running", "url", "http://localhost:"+port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) StartTLS(port, certFile, keyFile string) error {
	mux := s.setupMux()
	addr := ":" + port
	slog.Info("Dashboard running", "url", "https://localhost:"+port)
	return http.ListenAndServeTLS(addr, certFile, keyFile, mux)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"localhost", "127.0.0.1"},
	})
	if err != nil {
		slog.Error("websocket accept error", "error", err)
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

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	messages, err := s.store.QueryAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="mcpwatch_export.json"`)
	json.NewEncoder(w).Encode(messages)
}

func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	messages, err := s.store.QueryAll()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="mcpwatch_export.csv"`)

	writer := csv.NewWriter(w)
	defer writer.Flush()

	headers := []string{"id", "timestamp", "transport", "direction", "msg_type", "method", "jsonrpc_id", "latency_ms", "size_bytes", "token_estimate", "error_code", "params", "result", "error_data"}
	if err := writer.Write(headers); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, msg := range messages {
		row := []string{
			fmt.Sprintf("%d", msg.ID),
			msg.Timestamp.Format(time.RFC3339Nano),
			msg.Transport,
			msg.Direction,
			string(msg.MsgType),
			msg.Method,
			msg.JSONRPCID,
			fmt.Sprintf("%d", msg.LatencyMS),
			fmt.Sprintf("%d", msg.SizeBytes),
			fmt.Sprintf("%d", msg.TokenEstimate),
			msg.ErrorCode,
			msg.Params,
			msg.Result,
			msg.ErrorData,
		}
		if err := writer.Write(row); err != nil {
			return
		}
	}
}

func (s *Server) handleClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.store.Clear(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

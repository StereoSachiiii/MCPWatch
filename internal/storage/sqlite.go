package storage

import (
	"database/sql"
	"time"

	"mcpwatch/internal/engine"

	_ "modernc.org/sqlite"
)

// Store manages SQLite persistence for intercepted messages.
type Store struct {
	db *sql.DB
}

// New creates a new Store, initializes WAL mode and the schema.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	schema := `
	CREATE TABLE IF NOT EXISTS interactions (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp   DATETIME DEFAULT (strftime('%Y-%m-%dT%H:%M:%f', 'now')),
		transport   TEXT DEFAULT 'stdio',
		direction   TEXT,
		msg_type    TEXT,
		method      TEXT,
		jsonrpc_id  TEXT,
		params      TEXT,
		result      TEXT,
		error_data  TEXT,
		latency_ms  INTEGER DEFAULT 0,
		raw         TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_interactions_timestamp ON interactions(timestamp);
	CREATE INDEX IF NOT EXISTS idx_interactions_method ON interactions(method);
	CREATE INDEX IF NOT EXISTS idx_interactions_jsonrpc_id ON interactions(jsonrpc_id);
	`
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Insert writes a message to the database.
func (s *Store) Insert(msg *engine.Message) error {
	_, err := s.db.Exec(`
		INSERT INTO interactions (transport, direction, msg_type, method, jsonrpc_id, params, result, error_data, latency_ms, raw)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.Transport, msg.Direction, msg.MsgType, msg.Method,
		msg.JSONRPCID, msg.Params, msg.Result, msg.ErrorData,
		msg.LatencyMS, msg.Raw,
	)
	return err
}

// QueryRecent returns the most recent interactions, newest first.
func (s *Store) QueryRecent(limit int) ([]*engine.Message, error) {
	rows, err := s.db.Query(`
		SELECT id, timestamp, transport, direction, msg_type, method, jsonrpc_id,
		       params, result, error_data, latency_ms, raw
		FROM interactions
		ORDER BY id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []*engine.Message
	for rows.Next() {
		m := &engine.Message{}
		var ts string
		err := rows.Scan(&m.ID, &ts, &m.Transport, &m.Direction, &m.MsgType,
			&m.Method, &m.JSONRPCID, &m.Params, &m.Result,
			&m.ErrorData, &m.LatencyMS, &m.Raw)
		if err != nil {
			continue
		}
		if t, err := time.Parse("2006-01-02T15:04:05.000", ts); err == nil {
			m.Timestamp = t
		}
		messages = append(messages, m)
	}
	return messages, nil
}

// Stats holds aggregate metrics about intercepted traffic.
type Stats struct {
	TotalMessages int     `json:"total_messages"`
	TotalRequests int     `json:"total_requests"`
	TotalErrors   int     `json:"total_errors"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
}

// GetStats returns aggregate statistics from the database.
func (s *Store) GetStats() (*Stats, error) {
	st := &Stats{}
	err := s.db.QueryRow("SELECT COUNT(*) FROM interactions").Scan(&st.TotalMessages)
	if err != nil {
		return nil, err
	}
	s.db.QueryRow("SELECT COUNT(*) FROM interactions WHERE msg_type = 'request'").Scan(&st.TotalRequests)
	s.db.QueryRow("SELECT COUNT(*) FROM interactions WHERE error_data != '' AND error_data != 'null'").Scan(&st.TotalErrors)
	s.db.QueryRow("SELECT COALESCE(AVG(latency_ms), 0) FROM interactions WHERE latency_ms > 0").Scan(&st.AvgLatencyMS)
	return st, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

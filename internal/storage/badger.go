package storage

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"mcpwatch/internal/engine"

	"github.com/dgraph-io/badger/v4"
)


type Store struct {
	db        *badger.DB
	queue     chan *engine.Message
	idCounter atomic.Uint64
}


type Stats struct {
	TotalMessages int     `json:"total_messages"`
	TotalRequests int     `json:"total_requests"`
	TotalErrors   int     `json:"total_errors"`
	AvgLatencyMS  float64 `json:"avg_latency_ms"`
	TotalBytes    int64   `json:"total_bytes"`
	TotalTokens   int64   `json:"total_tokens"`
}


func New(path string) (*Store, error) {
	opts := badger.DefaultOptions(path)
	opts.Logger = nil 

	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	var initialCount uint64

	
	err = db.Update(func(txn *badger.Txn) error {
		var stats Stats
		item, err := txn.Get([]byte("stats"))
		if err == nil {
			item.Value(func(val []byte) error {
				return json.Unmarshal(val, &stats)
			})
			initialCount = uint64(stats.TotalMessages)
		} else if err == badger.ErrKeyNotFound {
			statsBytes, _ := json.Marshal(Stats{})
			return txn.Set([]byte("stats"), statsBytes)
		}
		return err
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	store := &Store{
		db:    db,
		queue: make(chan *engine.Message, 4096),
	}
	store.idCounter.Store(initialCount)
	go store.runDrain()

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_ = db.RunValueLogGC(0.5)
		}
	}()

	return store, nil
}


func (s *Store) Insert(msg *engine.Message) error {
	select {
	case s.queue <- msg:
	default:

	}
	return nil
}

func (s *Store) runDrain() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]*engine.Message, 0, 256)

	for {
		select {
		case msg := <-s.queue:
			batch = append(batch, msg)
			if len(batch) >= 256 {
				s.flush(batch)
				batch = batch[:0]
			}
		case <-ticker.C:
			if len(batch) > 0 {
				s.flush(batch)
				batch = batch[:0]
			}
		}
	}
}

func (s *Store) flush(batch []*engine.Message) {
	err := s.db.Update(func(txn *badger.Txn) error {

		var stats Stats
		item, err := txn.Get([]byte("stats"))
		if err == nil {
			item.Value(func(val []byte) error {
				return json.Unmarshal(val, &stats)
			})
		}

		
		var totalLatencyMS float64
		if stats.TotalMessages > 0 {
			totalLatencyMS = stats.AvgLatencyMS * float64(stats.TotalMessages)
		}

		for _, msg := range batch {
			msgID := s.idCounter.Add(1)
			msg.ID = int64(msgID)

			
			key := fmt.Sprintf("msg:%019d:%06d", msg.Timestamp.UnixNano(), msgID)
			val, err := json.Marshal(msg)
			if err != nil {
				continue
			}

			entry := badger.NewEntry([]byte(key), val).WithTTL(24 * time.Hour)
			if err := txn.SetEntry(entry); err != nil {
				return err
			}

			
			stats.TotalMessages++
			if msg.MsgType == engine.MsgTypeRequest {
				stats.TotalRequests++
			}
			if msg.ErrorData != "" && msg.ErrorData != "null" {
				stats.TotalErrors++
			}
			stats.TotalBytes += msg.SizeBytes
			stats.TotalTokens += msg.TokenEstimate
			if msg.LatencyMS > 0 {
				totalLatencyMS += float64(msg.LatencyMS)
			}
		}

		if stats.TotalMessages > 0 {
			stats.AvgLatencyMS = totalLatencyMS / float64(stats.TotalMessages)
		}

		statsBytes, _ := json.Marshal(stats)
		return txn.Set([]byte("stats"), statsBytes)
	})

	if err != nil {
		slog.Error("failed to flush to BadgerDB", "error", err)
	}
}


func (s *Store) QueryRecent(limit int) ([]*engine.Message, error) {
	var messages []*engine.Message

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = true
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("msg:")

		seekPrefix := append([]byte("msg:"), 0xFF)

		for it.Seek(seekPrefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var msg engine.Message
				if err := json.Unmarshal(val, &msg); err != nil {
					return err
				}
				messages = append(messages, &msg)
				return nil
			})
			if err != nil {
				slog.Error("failed to scan message row", "key", string(item.Key()), "error", err)
				continue
			}
			if len(messages) >= limit {
				break
			}
		}
		return nil
	})

	return messages, err
}


func (s *Store) GetStats() (*Stats, error) {
	var stats Stats
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte("stats"))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &stats)
		})
	})
	return &stats, err
}


func (s *Store) QueryAll() ([]*engine.Message, error) {
	var messages []*engine.Message

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Reverse = false
		it := txn.NewIterator(opts)
		defer it.Close()

		prefix := []byte("msg:")

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			err := item.Value(func(val []byte) error {
				var msg engine.Message
				if err := json.Unmarshal(val, &msg); err != nil {
					return err
				}
				messages = append(messages, &msg)
				return nil
			})
			if err != nil {
				continue
			}
		}
		return nil
	})

	return messages, err
}


func (s *Store) Clear() error {
	s.idCounter.Store(0)
	err := s.db.DropAll()
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		statsBytes, _ := json.Marshal(Stats{})
		return txn.Set([]byte("stats"), statsBytes)
	})
}


func (s *Store) Close() error {
	return s.db.Close()
}


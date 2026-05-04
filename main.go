package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	_ "modernc.org/sqlite"
)

type Interaction struct {
	Timestamp string `json:"timestamp"`
	Direction string `json:"direction"`
	Method    string `json:"method"`
	Params    string `json:"params"`
	Result    string `json:"result"`
	LatencyMS int64  `json:"latency_ms"`
	Raw       string `json:"raw"`
}

var (
	db       *sql.DB
	logChan  = make(chan *Interaction, 100)
)

func main() {
	wrapCmd := flag.String("wrap", "", "Command to wrap (e.g. \"node server.js\")")
	dbPath := flag.String("db", "mcpwatch.db", "Path to SQLite database")
	uiPort := flag.String("ui", "8080", "Port for the UI dashboard")
	flag.Parse()

	if *wrapCmd == "" {
		fmt.Println("Usage: mcpwatch --wrap \"command args\" [--ui port]")
		os.Exit(1)
	}

	initDB(*dbPath)
	defer db.Close()

	// Start background logger
	go backgroundLogger()

	// Start UI server
	go startUIServer(*uiPort)

	fmt.Fprintf(os.Stderr, "[MCPWatch] Wrapping: %s\n", *wrapCmd)
	fmt.Fprintf(os.Stderr, "[MCPWatch] UI Dashboard: http://localhost:%s\n", *uiPort)
	runStdioProxy(*wrapCmd)
}

func initDB(path string) {
	var err error
	db, err = sql.Open("sqlite", path)
	if err != nil {
		log.Fatal(err)
	}

	// Enable WAL mode for better concurrency
	_, err = db.Exec("PRAGMA journal_mode=WAL;")
	if err != nil {
		log.Fatal(err)
	}

	schema := `
	CREATE TABLE IF NOT EXISTS interactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		direction TEXT,
		method TEXT,
		params TEXT,
		result TEXT,
		latency_ms INTEGER,
		raw TEXT
	);`
	_, err = db.Exec(schema)
	if err != nil {
		log.Fatal(err)
	}
}

func backgroundLogger() {
	for i := range logChan {
		_, err := db.Exec(`
			INSERT INTO interactions (direction, method, params, result, raw)
			VALUES (?, ?, ?, ?, ?)`,
			i.Direction, i.Method, i.Params, i.Result, i.Raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[MCPWatch] DB Error: %v\n", err)
		}
	}
}

func runStdioProxy(commandStr string) {
	parts := strings.Fields(commandStr)
	cmd := exec.Command(parts[0], parts[1:]...)

	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}

	// Capture Client -> Server (stdin)
	go func() {
		reader := io.TeeReader(os.Stdin, stdin)
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			queueLog("IN", line)
		}
	}()

	// Capture Server -> Client (stdout)
	reader := io.TeeReader(stdout, os.Stdout)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := scanner.Text()
		queueLog("OUT", line)
	}

	cmd.Wait()
	close(logChan)
}

func queueLog(direction, raw string) {
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		return
	}

	method, _ := msg["method"].(string)
	params, _ := json.Marshal(msg["params"])
	result, _ := json.Marshal(msg["result"])

	logChan <- &Interaction{
		Direction: direction,
		Method:    method,
		Params:    string(params),
		Result:    string(result),
		Raw:       raw,
	}
}

func startUIServer(port string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "./web/index.html")
	})

	http.HandleFunc("/api/interactions", func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.Query("SELECT id, timestamp, direction, method, params, result, raw FROM interactions ORDER BY id DESC LIMIT 50")
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		defer rows.Close()

		var results []map[string]interface{}
		for rows.Next() {
			var id int
			var ts, dir, method, params, result, raw string
			rows.Scan(&id, &ts, &dir, &method, &params, &result, &raw)

			item := map[string]interface{}{
				"id":        id,
				"timestamp": ts,
				"direction": dir,
				"method":    method,
				"params":    json.RawMessage(params),
				"result":    json.RawMessage(result),
				"raw":       raw,
			}
			results = append(results, item)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	})

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

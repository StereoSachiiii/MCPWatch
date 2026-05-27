package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"mcpwatch/internal/engine"
	"mcpwatch/internal/server"
	"mcpwatch/internal/storage"
	"mcpwatch/internal/transport"
	"mcpwatch/web"
)

func main() {
	wrapCmd := flag.String("wrap", "", "Command to wrap (e.g. \"node server.js\")")
	proxyURL := flag.String("proxy", "", "Proxy to remote MCP server (e.g. \"http://localhost:3000\")")
	proxyPort := flag.String("proxy-port", "8081", "Local port to bind the proxy server to")
	pid := flag.Int("pid", 0, "Attach to existing process ID via eBPF")
	dbPath := flag.String("db", "mcpwatch.db", "Path to SQLite database")
	uiPort := flag.String("ui", "8080", "Port for the UI dashboard")
	flag.Parse()

	// 1. Validate arguments (only one transport mode allowed)
	modes := 0
	if *wrapCmd != "" {
		modes++
	}
	if *proxyURL != "" {
		modes++
	}
	if *pid != 0 {
		modes++
	}

	if modes != 1 {
		fmt.Println("Usage: mcpwatch [mode] [--db path] [--ui port]")
		fmt.Println("\nYou must specify EXACTLY ONE mode:")
		fmt.Println("  --wrap \"command args\"   Run and intercept stdio of a local command")
		fmt.Println("  --proxy \"url\"           Proxy and intercept HTTP/SSE to a remote server")
		fmt.Println("  --pid 1234              Attach and intercept via eBPF to an existing process (Linux only)")
		os.Exit(1)
	}

	// 2. Setup Context for Graceful Shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 3. Initialize Storage
	store, err := storage.New(*dbPath)
	if err != nil {
		log.Fatalf("[MCPWatch] failed to init database: %v", err)
	}
	defer store.Close()

	// 4. Initialize Correlator
	correlator := engine.NewCorrelator()

	// 5. Initialize Server (Hub and API)
	hub := server.NewHub()
	srv := server.New(store, hub, web.Assets)
	
	// Start server in background
	go func() {
		if err := srv.Start(*uiPort); err != nil {
			log.Printf("[MCPWatch] server stopped: %v", err)
		}
	}()

	// 6. Initialize Transport Handler
	var handler transport.Handler
	if *wrapCmd != "" {
		handler = transport.NewStdio(*wrapCmd)
	} else if *proxyURL != "" {
		handler = transport.NewProxy(*proxyURL, *proxyPort)
	} else if *pid != 0 {
		handler = transport.NewEBPF(*pid)
	}

	// 7. Start Transport
	messages := make(chan *engine.Message, 1000)
	errChan := make(chan error, 1)
	
	go func() {
		log.Printf("[MCPWatch] Starting transport: %s", handler.Type())
		errChan <- handler.Start(ctx, messages)
	}()

	// 8. The Central Event Loop
	log.Println("[MCPWatch] Core orchestrator running. Press Ctrl-C to stop.")
	for {
		select {
		case msg := <-messages:
			if msg == nil {
				continue
			}
			// Run Correlator logic
			correlator.Process(msg)
			
			// Store in Database
			if err := store.Insert(msg); err != nil {
				log.Printf("[MCPWatch] failed to insert message: %v", err)
			}

			// Broadcast to Web UI via WebSockets
			hub.Broadcast(msg)

		case err := <-errChan:
			if err != nil {
				log.Printf("[MCPWatch] Transport error: %v", err)
			}
			cancel() // trigger shutdown
			
		case <-ctx.Done():
			log.Println("\n[MCPWatch] Shutting down gracefully...")
			return
		}
	}
}

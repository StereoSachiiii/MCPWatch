# MCPWatch — The Universal Agent Inspector

MCPWatch is a transparent, non-intrusive observability proxy for the Model Context Protocol (MCP). It acts as a universal "Flight Recorder" for AI agents, capturing JSON-RPC interactions and providing a live, real-time inspection dashboard.

## Current Features (Phase 1)
- **Zero-Config Wrapping**: Wrap any existing MCP server (`mcpwatch --wrap "node server.js"`) to instantly monitor traffic without modifying the server.
- **Stdio Interception**: Transparently proxies `stdin` and `stdout` while capturing the JSON-RPC stream.
- **JSON-RPC Correlation**: Automatically correlates requests and responses via JSON-RPC IDs to calculate exact execution latency.
- **Persistent Audit Log**: All traffic is saved to a local SQLite database (`mcpwatch.db`) in WAL mode for high concurrency.
- **Cyber-Audit Dashboard**: A premium, real-time web dashboard (default: `http://localhost:8080`) built with glassmorphism aesthetics.
- **WebSocket Streaming**: Live push updates to the dashboard — see traffic as it happens.
- **Self-Contained**: The web UI is embedded directly into the Go binary. No external dependencies.

## Upcoming Features (Roadmap)
- **Phase 2 — Remote Transports**: Support for proxying Server-Sent Events (SSE) and HTTP-based MCP servers.
- **Phase 3 — eBPF Interception (Linux)**: Attach to already-running processes without restarting them using eBPF syscall interception (`sys_enter_write`, `sys_exit_read`).
- **Phase 4 — Advanced Analytics**: Deeper latency metrics, error rate tracking, and payload inspection tools.

## Setup
```bash
go mod tidy
go build -o mcpwatch.exe ./cmd/mcpwatch
```

## Usage
Wrap your server command:
```bash
mcpwatch.exe --wrap "node your_mcp_server.js" --ui 8080
```

Then open `http://localhost:8080` in your browser to monitor traffic in real time.

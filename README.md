# mcpwatch

A high-performance observability proxy for inspecting Model Context Protocol (MCP) traffic.

`mcpwatch` acts as a middleman. You run it wrapping a local MCP server command, proxying HTTP/SSE traffic, or attaching directly to already running Linux processes via eBPF. It intercepts the standard streams, records JSON-RPC messages, tracks request-response latencies, and exposes a beautiful web dashboard to inspect the traffic in real time.

---

## Features

*   **Three Capture Modes:** Stdio wrapper, HTTP/SSE reverse proxy, and Linux eBPF process tracer.
*   **Embedded Web Dashboard:** Single-binary deployment with flat-DOM HTML/CSS/JS embedded using `go embed` (no npm or Node.js required).
*   **BadgerDB Storage:** High-performance LSM-tree storage for message log auditing.
*   **Prometheus Metrics:** Native instrumentation for message counts, payload sizes, token estimates, and request latencies.
*   **Distributed Tracing:** OpenTelemetry context propagation and span creation for request-response flows.
*   **Active Monitoring & Alerts:** Webhook alerts (Slack-compatible) triggered by window-based error rate or latency breaches.
*   **Aggressive GC Tuning:** Programmatically tune heap growth targets to optimize execution footprint for bursty traffic.
*   **Security & TLS:** TLS server support with Basic Authentication for API and dashboard endpoints.

---

## Architecture

```
                  ┌─────────────────────────────────┐
                  │          MCP Client             │
                  └────────────────┬────────────────┘
                                   │
                Stdio / HTTP-SSE / eBPF Transport
                                   │
                                   ▼
                  ┌─────────────────────────────────┐
                  │            mcpwatch             │
                  │  ┌───────────────────────────┐  │
                  │  │     JSON-RPC Parser       │  │
                  │  └─────────────┬─────────────┘  │
                  │                │                │
                  │  ┌─────────────▼─────────────┐  │
                  │  │        Correlator         │  │
                  │  │   (Latency, OTel Spans)   │  │
                  │  └─────────────┬─────────────┘  │
                  │                │                │
                  │  ┌─────────────▼─────────────┐  │
                  │  │       Storage Hub         │  │
                  │  └─────┬───────────────┬─────┘  │
                  └────────┼───────────────┼────────┘
                           │               │
                           ▼               ▼
                     ┌───────────┐   ┌───────────┐
                     │ BadgerDB  │   │ Web Server│
                     └───────────┘   └───────────┘
```

---

## Build & Setup

### Prerequisites
*   Go 1.25.0 or later.
*   (Linux only) Clang/LLVM and `bpftool` if you plan to recompile the eBPF kernel C files.

### Compiling from Source

Run the following commands in the project root:

```bash
# Fetch and tidy dependencies
go mod tidy

# Build the executable
go build -o mcpwatch.exe .
```

### Running with Docker

You can build and run `mcpwatch` inside a container. The package contains a `Dockerfile` and a `docker-compose.yml` for quick composition:

```bash
docker-compose up --build
```

---

## Usage Guide & Command-Line Flags

Run `mcpwatch` specifying exactly **one** of the transport modes (`--wrap`, `--proxy`, or `--pid`):

```bash
Usage: mcpwatch [mode] [--db path] [--ui port] [other-flags]
```

### 1. Stdio Mode (Command Wrapper)
Wraps a local node/python executable, intercepting stdin/stdout streams:

```bash
mcpwatch.exe --wrap "node your_mcp_server.js" --ui 8080
```

### 2. HTTP/SSE Proxy Mode
Acts as a reverse proxy between your MCP client and a remote HTTP/SSE-based MCP server:

```bash
mcpwatch.exe --proxy "http://localhost:3000" --proxy-port 8081 --ui 8080
```

### 3. eBPF Mode (Linux Only)
Attaches to an already-running process by its PID without restarting or changing its configurations:

```bash
sudo ./mcpwatch --pid 28415 --ui 8080
```

---

## Configuration

### Command Line Reference

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--config` | `""` | Path to a JSON configuration file |
| `--wrap` | `""` | Command to run and wrap (Stdio mode) |
| `--proxy` | `""` | Remote MCP URL target (Proxy mode) |
| `--proxy-port`| `"8081"` | Local port to bind the proxy server |
| `--pid` | `0` | PID of the running process to trace via eBPF |
| `--db` | `"mcpwatch_data"` | Folder path to host the BadgerDB KV store |
| `--ui` | `"8080"` | Port to serve the dashboard interface |
| `--gc-percent` | `35` | Garbage collection heap target percentage (aggressive: `30` to `40`) |
| `--log-level` | `"info"` | Log granularity (`debug`, `info`, `warn`, `error`) |
| `--log-json` | `false` | Output console logs in JSON format |
| `--auth-user` | `""` | Username for basic auth on the dashboard |
| `--auth-pass` | `""` | Password for basic auth on the dashboard |
| `--tls-cert` | `""` | File path to TLS certificate for HTTPS |
| `--tls-key` | `""` | File path to TLS key for HTTPS |
| `--alert-webhook` | `""` | Alerting Webhook URL (Slack-compatible) |
| `--alert-error-rate`| `0.0` | Alert threshold for error rate (e.g. `10.0` for 10%) |
| `--alert-latency`| `0` | Alert threshold for latency in milliseconds |
| `--alert-window` | `60` | Duration window in seconds to analyze thresholds |

---

## Configuration File Snippet

Instead of passing flags, you can maintain configurations in a JSON file.

### `config.json`
```json
{
  "db": "data_store",
  "ui": "8443",
  "log_level": "debug",
  "log_json": true,
  "gc_percent": 35,
  "auth_user": "admin",
  "auth_pass": "SuperSecurePassword123",
  "tls_cert": "certs/server.crt",
  "tls_key": "certs/server.key",
  "alert_webhook": "https://hooks.slack.com/services/T00/B00/X00",
  "alert_error_rate": 15.5,
  "alert_latency": 1500,
  "alert_window": 120,
  "proxy": "http://localhost:3000",
  "proxy_port": "8081"
}
```

Run using the configuration file:
```bash
mcpwatch.exe --config config.json
```

---

## Developer Integration & Code Snippets

### 1. Active Goroutine Monitoring
The HTTP Web Server exposes a `/health` endpoint returning the health status alongside the current number of active goroutines managed by the Go runtime scheduler.

#### Endpoint Sample: `GET /health`
```json
{
  "active_goroutines": 14,
  "status": "ok",
  "timestamp": "2026-06-29T08:29:00+05:30"
}
```

### 2. Prometheus Metrics Scraping
Metrics are exposed at `GET /metrics` for ingestion into Prometheus:
```text
# HELP mcp_requests_total Total number of MCP requests.
# TYPE mcp_requests_total counter
mcp_requests_total{method="tools/list",status="success"} 42
mcp_requests_total{method="tools/call",status="error"} 2

# HELP mcp_request_duration_seconds Latency of MCP requests in seconds.
# TYPE mcp_request_duration_seconds histogram
mcp_request_duration_seconds_bucket{method="tools/call",le="0.1"} 10
mcp_request_duration_seconds_bucket{method="tools/call",le="0.5"} 35
mcp_request_duration_seconds_sum{method="tools/call"} 8.42
mcp_request_duration_seconds_count{method="tools/call"} 40
```

### 3. OpenTelemetry Distributed Tracing
OpenTelemetry context values are extracted from headers/payloads. Set the target trace collector endpoint using standard env variables:

```bash
$env:OTEL_EXPORTER_OTLP_ENDPOINT="http://localhost:4318"
./mcpwatch.exe --wrap "node server.js"
```

---

## Checked & Remaining TODOs

- [x] Integrate the internal packages with main.go. right now main.go is a separate monolith ignoring the internal folder.
- [x] Add the missing cli flags for proxy and ebpf modes.
- [x] Hook up the correlator to calculate request latency.
- [x] No graceful shutdown. if you ctrl-c the process, the sqlite database, child process, and websocket connections are not cleaned up properly.
- [x] Update remote proxy logic to use the current Streamable HTTP (ND-JSON) standard.
- [x] Fix the correlator memory leak. go maps don't shrink so bursty traffic will bloat the heap permanently.
- [x] Implement websocket streaming for live web ui updates.
- [x] Finish advanced analytics like error tracking and deep payload inspection.
- [x] Write and compile the actual eBPF C code for the kernel. the Go side is wired up but there is no tracer.c yet.
- [x] Abstract the parser behind a proper interface.
- [x] Zero test coverage. there are no unit tests for the correlator, parser, storage, or hub. need table-driven tests at minimum.
- [x] The messages channel in the transport handlers is unbuffered or has a fixed size. if the consumer is slow the sender goroutines will block silently and freeze the proxy.
- [x] No structured logging. everything uses raw fmt.Fprintf or log.Printf with no log levels. need at least debug/info/error levels.
- [x] No configuration file support. everything is hardcoded or passed as cli flags. should support a config file for complex setups.
- [x] No way to export or clear the database. users can't dump the audit log to json or csv, and can't reset it without deleting the file.
- [x] No authentication on the dashboard. anyone on the network can open port 8080 and see all intercepted traffic.
- [x] The web ui is served from the filesystem with http.ServeFile. it should be embedded into the binary using go embed so it ships as a single file.
- [x] No CI pipeline. no github actions for build, test, or release.
- [x] No versioning. the binary has no --version flag and no build-time version injection.
- [x] No health check endpoint. there is no way for monitoring systems to verify mcpwatch is alive.
- [x] The storage layer silently ignores scan errors in QueryRecent (line 98 just does continue). bad rows are dropped with no logging.
- [x] Sqlite writes are synchronous one-at-a-time inserts. should use a buffer + async drain pattern to batch inserts in a single transaction on a timer or size threshold. way faster, keeps the proxy path non-blocking.
- [x] Replace gorilla/websocket with nhooyr.io/websocket since gorilla is archived and nhooyr is smaller and natively supports context.
- [x] Make gc very aggressive, crank up to 30-40% heap target , check if this doesnt actualy because a bottleneck, learn how go GC tracks ownership and what makes something eligible for a gc run, make sure it doesnt negatively impact performance.
- [x] Find if coroutine explosion is actually a real thing, or if its just a myth made by people who are bad at writing go code, test it thoroughly. find if the coroutine can use mutexes and how does that userspace scheduler  cooperate with linux kernel scheduler to prevent coroutine starvation. also find if there is a way to get the number of coroutines in the system. if there isnt, make one. create a utility or something to get the number of coroutines. does # of coroutines reflect the current workload? how does that work> solidify it.
- [x] Why is go using SIGURG??? how does that map into my code.
- [ ] Dont have to know everything, just what happens to my data , how it gets bounced/ moved/copied around the userspace and who owns the resources, how its scheduled , why is  badger  beyond just memory buffer and kv architecture
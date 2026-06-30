# mcpwatch

A high-performance observability proxy for inspecting Model Context Protocol (MCP) traffic.

`mcpwatch` acts as a middleman. You run it wrapping a local MCP server command, proxying HTTP/SSE traffic, or attaching directly to already running Linux processes via eBPF. It intercepts the standard streams, records JSON-RPC messages, tracks request-response latencies, and exposes a beautiful web dashboard to inspect the traffic in real time.

---

## Features

*   **Four Capture Modes:** Stdio wrapper, HTTP/SSE reverse proxy, Linux eBPF process tracer, and Unix Domain Socket (UDS) proxy.
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

Run `mcpwatch` specifying exactly **one** of the transport modes (`--wrap`, `--proxy`, `--pid`, or `--socket-local`/`--socket-target`):

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

### 4. Unix Domain Socket (UDS) Mode
Acts as a local Unix domain socket proxy, forwarding requests to the target socket:

```bash
mcpwatch.exe --socket-local "/tmp/mcp.sock" --socket-target "/tmp/real_mcp.sock" --ui 8080
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
| `--socket-local`| `""` | Local Unix socket path to listen on (Socket mode) |
| `--socket-target`| `""` | Target Unix socket path to forward to (Socket mode) |
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

## Technical & Conceptual Deep-Dive

### 1. Data Lifecycles: Copies, Heap Escape, and Memory Ownership
When an MCP message is processed by `mcpwatch`, data moves through distinct user-space memory states:
*   **Kernel to User Space:** Operating system sockets or pipe handles buffer incoming TCP/stdio streams. When `mcpwatch` reads from these descriptors (`Reader.Read`), the kernel copies network buffers into Go-allocated byte slices.
*   **Buffer Allocation & Copies:** In `streamInterceptor` or the stdio scanner, bytes are read into stack-allocated slices. When we split inputs by newline delimiters (`\n`), new slices referencing the underlying array are formed. If these slices are passed to goroutines, written to channels (`messages`), or converted to strings (e.g. `string(lineBytes)`), Go copies the slice contents into newly allocated heap objects, since the string lifecycle extends beyond the scanner's stack frame.
*   **Heap Escape Analysis:** The Go compiler runs escape analysis to determine if variables can remain on the stack or must be moved to the heap. If a byte slice escapes (e.g. parsed into a JSON-RPC struct pointer returned by a function, or sent across a channel to another goroutine), it triggers a heap allocation. Heap allocations require garbage collection. The aggressive GC tuning (`--gc-percent 35`) tells the Go runtime to trigger a GC sweep once the live heap grows by 35% over the base heap size, limiting memory growth under high, bursty JSON parsing workloads.

### 2. The Go Runtime Scheduler & Synchronization
Go schedules concurrency using the GMP model:
*   **G (Goroutine):** Represents the goroutine structure, containing stack pointers, PC, and execution state.
*   **M (Machine/OS Thread):** An actual OS thread managed by the kernel scheduler.
*   **P (Processor):** A logical processor representing resource execution contexts. The number of Ps defaults to GOMAXPROCS.
*   **Preemption and `SIGURG`:** Prior to Go 1.14, scheduling was purely cooperative (goroutines only yielded at function calls or allocation boundaries). A tight, non-allocating loop could lock a thread indefinitely. To solve this, Go introduced asynchronous preemption. The runtime periodically checks goroutine runtimes. If a goroutine runs for too long (>10ms), the system thread sends a **`SIGURG` (Urgent Out-of-Band Data)** signal to the thread executing it. The thread's signal handler intercepts `SIGURG`, saves the register state, and calls `goschedimpl` to park the goroutine and run another G, preventing CPU starvation.
*   **Mutex Starvation & Futexes:** When Go goroutines synchronize using `sync.Mutex`, they attempt to acquire a lock via fast-path atomic updates. If the lock is held, the goroutine parks on a semaphore queue using kernel `futex` (fast userspace mutex) calls. Go mutexes feature a **starvation mode** to prevent lock acquisition starvation by active CPU goroutines. If a waiting goroutine fails to acquire the lock for more than 1 millisecond, it flags the mutex as starved. In starvation mode, the lock is handed directly to the first goroutine in the wait queue, bypassing the fast-path acquisition of newly spawned running goroutines.

### 3. BadgerDB Architecture: WiscKey and the LSM-Tree
Traditional Key-Value stores (like RocksDB or LevelDB) use a Log-Structured Merge-tree (LSM-tree) where keys and values are stored together. During compaction (merging and sorting SSTables across disk tiers), both keys and values are rewritten repeatedly. This causes high **Write Amplification**, wearing out SSDs and saturating I/O.

BadgerDB resolves this using the **WiscKey** architecture:
*   **Key-Value Separation:** Keys are kept in a standard memory-buffered LSM-tree (using SSTables) for fast binary search, indexing, and range queries. Values, which are typically much larger, are appended sequentially to a separate **Value Log (Vlog)**.
*   **Compaction Efficiency:** During compaction runs, BadgerDB only sorts and rewrites SSTables containing the keys. It does not rewrite the values in the Vlog, slashing write amplification factors from ~10-30x down to nearly 1.1-2x.
*   **Garbage Collection:** As keys are deleted or updated, their values in the Vlog become stale. BadgerDB runs background Vlog garbage collection by reading blocks sequentially, checking if the corresponding key still exists in the LSM-tree, and rewriting valid keys to the tail of the log while discarding stale space.
*   **Memory-Mapped Files (mmap):** BadgerDB utilizes standard kernel memory mapping (`mmap`) to map database SSTables directly into user space. This allows the OS virtual memory manager to handle file caching natively, eliminating user-space copying and context-switching overhead during reads.
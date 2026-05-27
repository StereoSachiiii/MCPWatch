mcpwatch

a proxy for inspecting Model Context Protocol traffic.

how it works:
it acts as a middleman. you run mcpwatch wrapping your normal server command. it intercepts the standard input and output streams, records the json-rpc messages, and pipes them through untouched. messages are saved to a local sqlite database. a web interface reads from this database to show you what happened.

architecture:
- a stdio handler for local processes
- a remote proxy handler for streamable http (nd-json)
- an ebpf handler for attaching to already-running linux processes
- a correlator to match requests with responses and track latency

setup:
go mod tidy
go build -o mcpwatch.exe .

usage:
mcpwatch.exe --wrap "node your_mcp_server.js" --ui 8080

open http://localhost:8080 to see the traffic.

whats done:
we have a basic cli wrapper for intercepting standard input and output.
we are successfully saving all intercepted json messages to a local sqlite database.
we have a simple web ui that serves an api endpoint showing recent traffic.
we have the foundation for our correlator and modular transports in the internal folder.

frontend philosophy:
no frameworks. plain html, css, js only. keep the dom as flat as possible. manage state up top in js and push updates directly to the dom. this keeps rendering fast for real-time websocket traffic and avoids any build pipeline. the entire ui ships embedded in the go binary as a single file.

toolchain:
- go 1.22
- modernc.org/sqlite (pure go sqlite driver, no cgo needed)
- nhooyr.io/websocket (minimal websocket server for live dashboard updates)
- cilium/ebpf (loading and attaching ebpf programs from go, linux only)
- clang + llvm (compiling the ebpf C code to bytecode, linux only)
- go embed (embedding the web ui into the binary)
- no node, no npm, no frontend build step

todos:
- [x] integrate the internal packages with main.go. right now main.go is a separate monolith ignoring the internal folder.
- [x] add the missing cli flags for proxy and ebpf modes.
- [x] hook up the correlator to calculate request latency.
- [x] no graceful shutdown. if you ctrl-c the process, the sqlite database, child process, and websocket connections are not cleaned up properly.
- [ ] update remote proxy logic to use the current Streamable HTTP (ND-JSON) standard.
- [ ] fix the correlator memory leak. go maps don't shrink so bursty traffic will bloat the heap permanently.
- [ ] implement websocket streaming for live web ui updates.
- [ ] finish advanced analytics like error tracking and deep payload inspection.
- [ ] write and compile the actual eBPF C code for the kernel. the Go side is wired up but there is no tracer.c yet.
- [ ] abstract the parser behind a proper interface.
- [ ] zero test coverage. there are no unit tests for the correlator, parser, storage, or hub. need table-driven tests at minimum.
- [ ] the messages channel in the transport handlers is unbuffered or has a fixed size. if the consumer is slow the sender goroutines will block silently and freeze the proxy.
- [ ] no structured logging. everything uses raw fmt.Fprintf or log.Printf with no log levels. need at least debug/info/error levels.
- [ ] no configuration file support. everything is hardcoded or passed as cli flags. should support a config file for complex setups.
- [ ] no way to export or clear the database. users can't dump the audit log to json or csv, and can't reset it without deleting the file.
- [ ] no authentication on the dashboard. anyone on the network can open port 8080 and see all intercepted traffic.
- [x] the web ui is served from the filesystem with http.ServeFile. it should be embedded into the binary using go embed so it ships as a single file.
- [ ] no CI pipeline. no github actions for build, test, or release.
- [ ] no versioning. the binary has no --version flag and no build-time version injection.
- [ ] no health check endpoint. there is no way for monitoring systems to verify mcpwatch is alive.
- [ ] the storage layer silently ignores scan errors in QueryRecent (line 98 just does continue). bad rows are dropped with no logging.
- [ ] sqlite writes are synchronous one-at-a-time inserts. should use a buffer + async drain pattern to batch inserts in a single transaction on a timer or size threshold. way faster, keeps the proxy path non-blocking.
- [ ] replace gorilla/websocket with nhooyr.io/websocket since gorilla is archived and nhooyr is smaller and natively supports context.

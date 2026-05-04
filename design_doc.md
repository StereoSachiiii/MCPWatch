MCPWATCH: THE UNIVERSAL AGENT INSPECTOR
=========================================
System Design Document (V1.0 - Transport Agnostic)

1. INTRODUCTION
---------------
MCPWatch is a transparent, non-intrusive observability framework for the Model Context Protocol (MCP). It acts as a universal "Flight Recorder" for AI agents, capturing all JSON-RPC interactions regardless of the underlying transport layer.


2. THE "DEFINITION OF DONE" (DoD)
---------------------------------
MCPWatch is considered complete when it meets the following criteria:

- Universal Transport Support: Transparently proxies MCP servers over Stdio, SSE, and HTTP.
- Zero Configuration: Functions as a drop-in replacement for the original server command.
- Persistent Audit Log: Every request and response is logged to a local SQLite database with millisecond-precision timestamps.
- Honest Visualization: A premium, functional dashboard providing a unified timeline of interactions across all transports.
- No Interference: Functions purely as an observer with zero impact on agent/server behavior.


3. CORE ARCHITECTURE: THE UNIVERSAL INTERCEPTOR
-----------------------------------------------

A. Transport Abstraction Layer:
Instead of a single proxy, MCPWatch uses a modular transport system:
- Stdio Handler (Pipes): Wraps local processes, intercepting stdin and stdout.
- SSE Handler (Server-Sent Events): Proxies persistent HTTP streams for remote servers.
- HTTP Handler: Proxies standard request/response JSON-RPC over web protocols.

B. The Black Box (SQLite Storage):
A unified schema that decouples "What was said" from "How it was sent."
Columns: timestamp, transport, direction, method, payload, latency_ms.

C. The Audit Dashboard:
A self-contained HTML/JS dashboard that provides:
- Unified Timeline: A chronological view of agent activity.
- Deep-Dive Inspector: Formatted JSON views for arguments and results.
- Transport Transparency: Visual indicators of the connection type.


4. TECHNICAL WORKFLOW
---------------------
1. Proxy Initialization:
   - Stdio: mcpwatch --wrap "node server.js"
   - SSE/HTTP: mcpwatch --proxy "http://remote-mcp-server"
2. Interception: The relevant TransportHandler begins piping JSON-RPC chunks.
3. Passive Capture: Chunks are forwarded immediately to ensure zero latency, while copies are queued for logging.
4. Correlation: The system matches JSON-RPC ids to calculate exact tool execution times.
5. Serialization: Data is persisted to mcpwatch.db.
6. Audit: The user views the dashboard.html to review the agent's behavior.


5. FOLDER STRUCTURE
-------------------
mcpwatch/
|-- bin/
|   `-- mcpwatch.js          (CLI Entry / Router)
|-- core/
|   |-- transport/           (Modular Handlers: stdio.js, sse.js, http.js)
|   |-- db.js                (Unified SQLite logging)
|   `-- correlation.js       (JSON-RPC parsing & latency)
|-- web/
|   |-- index.html           (Timeline UI)
|   |-- styles.css           (Aesthetics)
|   `-- app.js               (UI Logic)
|-- mcpwatch.db              (Audit Log)
|-- package.json
`-- README.md


6. DESIGN AESTHETICS: "CYBER-AUDIT"
-----------------------------------
- Palette: Deep Charcoal, Slate Gray, and Electric Cyan.
- Typography: Inter (UI) and JetBrains Mono (Code).
- Components: Glassmorphism timeline cards, real-time data streaming, and high-contrast status badges.

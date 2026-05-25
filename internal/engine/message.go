package engine

import "time"

// Message represents a parsed JSON-RPC interaction intercepted by MCPWatch.
type Message struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Transport string    `json:"transport"`  // stdio, sse, http, ebpf
	Direction string    `json:"direction"`  // IN (client→server), OUT (server→client)
	MsgType   string    `json:"msg_type"`   // request, response, notification
	Method    string    `json:"method"`
	JSONRPCID string    `json:"jsonrpc_id"`
	Params    string    `json:"params"`
	Result    string    `json:"result"`
	ErrorData string    `json:"error_data"`
	LatencyMS int64     `json:"latency_ms"`
	Raw       string    `json:"raw"`
}

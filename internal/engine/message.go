package engine

import "time"


type MsgType string

const (
	MsgTypeRequest      MsgType = "request"
	MsgTypeResponse     MsgType = "response"
	MsgTypeNotification MsgType = "notification"
	MsgTypeStderr       MsgType = "stderr"
)


type Message struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Transport string    `json:"transport"`  
	Direction string    `json:"direction"`  
	MsgType   MsgType   `json:"msg_type"`   
	Method    string    `json:"method"`
	JSONRPCID string    `json:"jsonrpc_id"`
	Params    string    `json:"params"`
	Result    string    `json:"result"`
	ErrorData string    `json:"error_data"`
	LatencyMS     int64     `json:"latency_ms"`
	SizeBytes     int64     `json:"size_bytes"`
	TokenEstimate int64     `json:"token_estimate"`
	ErrorCode     string    `json:"error_code"`
	Raw           string    `json:"raw"`
}

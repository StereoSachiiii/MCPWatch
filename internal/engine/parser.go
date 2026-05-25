package engine

import (
	"encoding/json"
	"fmt"
	"time"
)

// ParseJSONRPC attempts to parse a raw line as a JSON-RPC 2.0 message.
// Returns nil if the line is not valid JSON.
func ParseJSONRPC(raw, direction, transport string) *Message {
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return nil
	}

	msg := &Message{
		Timestamp: time.Now(),
		Transport: transport,
		Direction: direction,
		Raw:       raw,
	}

	// Extract method
	if method, ok := obj["method"].(string); ok {
		msg.Method = method
	}

	// Extract JSON-RPC id (can be string or number)
	if id, ok := obj["id"]; ok && id != nil {
		msg.JSONRPCID = fmt.Sprintf("%v", id)
	}

	// Extract params
	if params, ok := obj["params"]; ok {
		data, _ := json.Marshal(params)
		msg.Params = string(data)
	}

	// Extract result
	if result, ok := obj["result"]; ok {
		data, _ := json.Marshal(result)
		msg.Result = string(data)
	}

	// Extract error
	if errData, ok := obj["error"]; ok {
		data, _ := json.Marshal(errData)
		msg.ErrorData = string(data)
	}

	// Classify message type
	switch {
	case msg.Method != "" && msg.JSONRPCID != "":
		msg.MsgType = "request"
	case msg.Method != "" && msg.JSONRPCID == "":
		msg.MsgType = "notification"
	case msg.JSONRPCID != "":
		msg.MsgType = "response"
	}

	return msg
}

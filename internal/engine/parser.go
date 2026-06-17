package engine

import (
	"encoding/json"
	"fmt"
	"time"
)



type Parser interface {
	Parse(raw, direction, transport string) *Message
}

type JSONRPCParser struct{}

func NewJSONRPCParser() Parser {
	return &JSONRPCParser{}
}

func (p *JSONRPCParser) Parse(raw, direction, transport string) *Message {
	return ParseJSONRPC(raw, direction, transport)
}

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

	
	if method, ok := obj["method"].(string); ok {
		msg.Method = method
	}

	
	if id, ok := obj["id"]; ok && id != nil {
		msg.JSONRPCID = fmt.Sprintf("%v", id)
	}

	
	if params, ok := obj["params"]; ok {
		data, _ := json.Marshal(params)
		msg.Params = string(data)
	}

	
	if result, ok := obj["result"]; ok {
		data, _ := json.Marshal(result)
		msg.Result = string(data)
	}

	
	if errData, ok := obj["error"]; ok {
		data, _ := json.Marshal(errData)
		msg.ErrorData = string(data)
		if errMap, ok := errData.(map[string]interface{}); ok {
			if code, ok := errMap["code"]; ok {
				msg.ErrorCode = fmt.Sprintf("%v", code)
			}
		}
	}

	
	msg.SizeBytes = int64(len(raw))
	msg.TokenEstimate = msg.SizeBytes / 4 

	
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

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err == nil {
			switch req.Method {
			case "tools/list":
				sendResponse(req.ID, map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "calculate",
							"description": "Calculate an expression",
							"inputSchema": map[string]interface{}{"type": "object"},
						},
					},
				})
			case "tools/call":
				var p ToolCallParams
				json.Unmarshal(req.Params, &p)
				if p.Name == "calculate" {
					sendResponse(req.ID, map[string]interface{}{
						"content": []map[string]interface{}{
							{"type": "text", "text": "The answer is 42"},
						},
					})
				}
			default:
				sendResponse(req.ID, map[string]interface{}{"status": "ok"})
			}
		}
	}
}

func sendResponse(id interface{}, result interface{}) {
	resp := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

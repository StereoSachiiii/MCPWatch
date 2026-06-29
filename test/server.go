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
			case "initialize":
				sendResponse(req.ID, map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]interface{}{
						"tools": map[string]interface{}{},
					},
					"serverInfo": map[string]interface{}{
						"name":    "calculator-server",
						"version": "1.0.0",
					},
				})
			case "tools/list":
				sendResponse(req.ID, map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "calculate",
							"description": "Calculate an expression",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"expression": map[string]interface{}{
										"type":        "string",
										"description": "The math expression to evaluate (e.g. '15 + 27')",
									},
								},
								"required": []string{"expression"},
							},
						},
					},
				})
			case "tools/call":
				var p ToolCallParams
				json.Unmarshal(req.Params, &p)
				if p.Name == "calculate" {
					var args struct {
						Expression string `json:"expression"`
					}
					json.Unmarshal(p.Arguments, &args)
					exprText := args.Expression
					if exprText == "" {
						exprText = "the expression"
					}
					sendResponse(req.ID, map[string]interface{}{
						"content": []map[string]interface{}{
							{"type": "text", "text": fmt.Sprintf("Calculated '%s'. The answer is 42.", exprText)},
						},
					})
				}
			default:
				if req.ID != nil {
					sendResponse(req.ID, map[string]interface{}{"status": "ok"})
				}
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

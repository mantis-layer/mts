// Command mcpserver 是一个最小 MCP stdio server（测试用）：
// 处理 initialize / notifications/initialized / tools/list / tools/call。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		if req.Method == "notifications/initialized" {
			continue
		}
		var resp response
		resp.JSONRPC = "2.0"
		resp.ID = req.ID
		switch req.Method {
		case "initialize":
			resp.Result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "fake-mcp", "version": "0.1.0"},
			}
		case "tools/list":
			resp.Result = map[string]any{
				"tools": []map[string]any{
					{
						"name":        "echo",
						"description": "回显输入文本",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"text": map[string]any{"type": "string"},
							},
							"required": []string{"text"},
						},
					},
				},
			}
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			text, _ := params.Arguments["text"].(string)
			if text == "boom" {
				resp.Error = &struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				}{Code: -32000, Message: "internal error: boom"}
				break
			}
			resp.Result = map[string]any{
				"content": []map[string]any{
					{"type": "text", "text": "echo: " + text},
				},
				"isError": false,
			}
		default:
			resp.Error = &struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			}{Code: -32601, Message: "method not found: " + req.Method}
		}
		payload, err := json.Marshal(resp)
		if err != nil {
			continue
		}
		fmt.Println(string(payload))
	}
}

package mcp

import (
	"context"
	"fmt"

	agentcore "github.com/mantis-layer/mts/agent-core"
)

// ToolAdapter 把 MCP server 的一个工具包装为 agent-core Tool（FR-006）。
type ToolAdapter struct {
	client *Client
	info   ToolInfo
	name   string
}

// NewToolAdapter 从 ToolInfo 构造 ToolAdapter。name 为空时使用 info.Name。
func NewToolAdapter(client *Client, info ToolInfo, name string) *ToolAdapter {
	if name == "" {
		name = info.Name
	}
	return &ToolAdapter{client: client, info: info, name: name}
}

// Name 返回工具唯一 ID。
func (a *ToolAdapter) Name() string { return a.name }

// Description 返回工具描述。
func (a *ToolAdapter) Description() string { return a.info.Description }

// Parameters 返回输入 JSON Schema（MCP inputSchema，可能为 nil）。
func (a *ToolAdapter) Parameters() map[string]any {
	if a.info.InputSchema == nil {
		// 缺省宽松 object，避免 Schema 校验误拒
		return map[string]any{"type": "object"}
	}
	return a.info.InputSchema
}

// Execute 通过 MCP tools/call 调用远端工具。
func (a *ToolAdapter) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	text, isError, err := a.client.CallTool(ctx, a.info.Name, input)
	if err != nil {
		return nil, fmt.Errorf("mcp tool %s: %w", a.info.Name, err)
	}
	if isError {
		return nil, agentcore.NewToolError("mcp_tool_error", "MCP 工具返回错误: "+text)
	}
	return map[string]any{"content": text}, nil
}

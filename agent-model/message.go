// Package agentmodel 定义统一的模型抽象层：消息、工具 Schema、
// 模型接口、流式事件、Usage 与结构化错误。
//
// 该包不依赖 Agent Runtime，也不依赖任何具体厂商 SDK。
package agentmodel

// Role 标识消息的发言者。
type Role string

const (
	// RoleSystem 系统提示。
	RoleSystem Role = "system"
	// RoleUser 用户输入。
	RoleUser Role = "user"
	// RoleAssistant 模型回复。
	RoleAssistant Role = "assistant"
	// RoleTool 工具执行结果。
	RoleTool Role = "tool"
)

// ToolCall 表示模型请求调用一个工具。
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 编码的参数对象
}

// Message 是模型与 Agent 之间交换的一条消息。
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolCallIDs 返回该消息包含的所有工具调用 ID。
func (m Message) ToolCallIDs() []string {
	ids := make([]string, 0, len(m.ToolCalls))
	for _, tc := range m.ToolCalls {
		ids = append(ids, tc.ID)
	}
	return ids
}

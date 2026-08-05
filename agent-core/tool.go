// Package agentcore 提供最小 Agent 循环运行时：Agent State、
// Model↔Tool 循环、结构化事件流、取消、Steering 与 Context Hook。
//
// 该包不负责 Task 生命周期与持久化（属于 agent-runtime，后续切片）。
package agentcore

import (
	"context"
	"fmt"
)

// Tool 是 Agent 可调用能力的最小契约。
type Tool interface {
	// Name 返回唯一工具 ID（模型通过它发起调用）。
	Name() string
	// Description 供模型理解何时调用。
	Description() string
	// Parameters 返回输入 JSON Schema。
	Parameters() map[string]any
	// Execute 执行工具；input 已通过 Schema 校验。
	Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

// ToolError 是结构化工具错误。
type ToolError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("tool %s: %s", e.Code, e.Message)
}

// NewToolError 构造结构化工具错误。
func NewToolError(code, message string) *ToolError {
	return &ToolError{Code: code, Message: message}
}

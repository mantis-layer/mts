package agentcontract

import (
	"context"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// ContextBuilder 将相关记忆注入当前消息视图（FR-012）。
// 在 Steering 之后、模型调用之前执行；输入 Persona + MemoryStore + 当前消息，
// 返回注入相关记忆后的消息视图。
type ContextBuilder interface {
	// Build 组装注入相关记忆后的消息。
	Build(ctx context.Context, p Persona, store MemoryStore, msg agentmodel.Message) (agentmodel.Message, error)
}

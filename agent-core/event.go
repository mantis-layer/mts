package agentcore

import (
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// EventKind 标识 Agent 运行时事件类型。
type EventKind string

const (
	// EventRunStart 一次 Run 开始。
	EventRunStart EventKind = "run.start"
	// EventModelStart 模型调用开始。
	EventModelStart EventKind = "model.start"
	// EventModelDelta 模型流式文本增量。
	EventModelDelta EventKind = "model.delta"
	// EventModelDone 一次模型调用结束（携带 Usage）。
	EventModelDone EventKind = "model.done"
	// EventToolStart 工具调用开始。
	EventToolStart EventKind = "tool.start"
	// EventToolDone 工具调用成功（携带结果）。
	EventToolDone EventKind = "tool.done"
	// EventToolError 工具调用失败（结构化错误）。
	EventToolError EventKind = "tool.error"
	// EventAgentMessage Agent 产出最终消息。
	EventAgentMessage EventKind = "agent.message"
	// EventAgentError Agent 运行错误。
	EventAgentError EventKind = "agent.error"
	// EventMemoryInjected ContextBuilder 完成一次记忆注入（FR-012 / S14）。
	// Content 携带注入的消息内容；Error 非 nil 表示检索失败已降级为不注入。
	EventMemoryInjected EventKind = "memory.injected"
	// EventRunEnd 一次 Run 结束。
	EventRunEnd EventKind = "run.end"
)

// Event 是一次 Agent 运行过程中的结构化事件（FR-002 Event Stream）。
type Event struct {
	Kind       EventKind
	Timestamp  time.Time
	Model      string
	Tool       string
	ToolCallID string
	Content    string
	Usage      *agentmodel.Usage
	Error      error
}

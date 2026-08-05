package agentmodel

import "context"

// FinishReason 表示模型停止生成的原因。
type FinishReason string

const (
	// FinishReasonStop 正常完成。
	FinishReasonStop FinishReason = "stop"
	// FinishReasonToolCalls 模型请求调用工具。
	FinishReasonToolCalls FinishReason = "tool_calls"
	// FinishReasonLength 因长度限制截断。
	FinishReasonLength FinishReason = "length"
	// FinishReasonContentFilter 内容过滤。
	FinishReasonContentFilter FinishReason = "content_filter"
)

// Usage 记录一次模型调用的 token 消耗。
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Request 是一次模型补全的输入。
type Request struct {
	Messages []Message    `json:"messages"`
	Tools    []ToolSchema `json:"tools,omitempty"`
	Model    string       `json:"model,omitempty"`
}

// Response 是一次非流式模型调用的结果。
type Response struct {
	Message      Message
	Usage        Usage
	FinishReason FinishReason
}

// StreamEventKind 标识流式事件类型。
type StreamEventKind string

const (
	// StreamEventDelta 文本增量。
	StreamEventDelta StreamEventKind = "delta"
	// StreamEventToolCall 一个完整（组装完成）的工具调用。
	StreamEventToolCall StreamEventKind = "tool_call"
	// StreamEventUsage token 用量。
	StreamEventUsage StreamEventKind = "usage"
	// StreamEventFinish 结束原因。
	StreamEventFinish StreamEventKind = "finish"
	// StreamEventError 流中发生错误。
	StreamEventError StreamEventKind = "error"
)

// StreamEvent 是一次流式补全过程中的增量事件。
type StreamEvent struct {
	Kind         StreamEventKind
	Delta        string
	ToolCall     *ToolCall
	Usage        *Usage
	FinishReason FinishReason
	Error        error
}

// Model 抽象统一模型补全能力。具体 Provider（OpenAI 兼容端点等）实现该接口。
type Model interface {
	// Complete 执行一次非流式补全。
	Complete(ctx context.Context, req Request) (Response, error)
	// Stream 执行一次流式补全，逐段返回增量事件。
	// 调用方必须消费完通道；流结束时通道关闭。
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

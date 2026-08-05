# agent-model — Provider 无关模型契约

`agent-model` 是**零内部依赖**的纯契约层：它定义消息、工具 Schema、流式事件、Usage 与 `Model` 接口，但不包含任何 HTTP 实现。任何 Provider（OpenAI 兼容、Anthropic、本地服务等）都以实现 `Model` 接口的方式接入。

模块路径：`github.com/mantis-layer/mts/agent-model`

## 设计原理

- **抽象在底层**：`agent-core` / `agent-runtime` 只依赖 `agent-model`，不依赖任何厂商 SDK。
- **流式是一等公民**：`Stream` 返回 `<-chan StreamEvent`，模型增量、Tool Call 分片、Usage 与 Finish 都以事件表达，调用方（Agent Loop）统一消费。
- **结构化错误**：`ModelError` 带 `Kind` 分类与 `Retryable` 标记，上层据此做重试/降级/告警，而不是解析字符串。
- **Tool Call 是数据不是魔法**：模型输出 `ToolCall{ID, Name, Arguments}`，参数是 JSON 字符串，由上层校验与执行——本模块不解析、不执行。

## 类型与接口

### `Role` 与 `Message`

```go
type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

type ToolCall struct {
    ID        string `json:"id"`
    Name      string `json:"name"`
    Arguments string `json:"arguments"` // JSON 编码的参数对象
}

type Message struct {
    Role       Role       `json:"role"`
    Content    string     `json:"content,omitempty"`
    ToolCallID string     `json:"tool_call_id,omitempty"` // RoleTool 时回填
    Name       string     `json:"name,omitempty"`         // RoleTool 时回填
    ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // RoleAssistant 时携带
}
```

`RoleTool` 消息通过 `ToolCallID` 关联工具调用结果；`RoleAssistant` 消息通过 `ToolCalls` 携带模型请求调用的工具。

### `ToolSchema` / `FunctionSchema`

```go
type ToolSchema struct {
    Type     string         `json:"type"`     // 恒为 "function"
    Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Parameters  map[string]any `json:"parameters"` // JSON Schema（object）
}
```

### `Model` 接口

```go
type Model interface {
    // Complete 执行一次非流式补全。
    Complete(ctx context.Context, req Request) (Response, error)
    // Stream 执行一次流式补全，逐段返回增量事件。
    Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
}

type Request struct {
    Messages []Message   `json:"messages"`
    Tools    []ToolSchema `json:"tools,omitempty"`
    Model    string      `json:"model,omitempty"`
    Stream   bool        `json:"-"`
}

type Response struct {
    Message      Message
    Usage        Usage
    FinishReason FinishReason
}

type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

type FinishReason string

const (
    FinishReasonStop          FinishReason = "stop"
    FinishReasonToolCalls     FinishReason = "tool_calls"
    FinishReasonLength        FinishReason = "length"
    FinishReasonContentFilter FinishReason = "content_filter"
)
```

### `StreamEvent`

```go
type StreamEventKind string

const (
    StreamEventDelta    StreamEventKind = "delta"    // 文本增量
    StreamEventToolCall StreamEventKind = "tool_call" // Tool Call（分片组装后完整）
    StreamEventUsage    StreamEventKind = "usage"     // token 用量
    StreamEventFinish   StreamEventKind = "finish"    // 结束 + FinishReason
    StreamEventError    StreamEventKind = "error"     // 流中途错误
)

type StreamEvent struct {
    Kind         StreamEventKind
    Delta        string
    ToolCall     *ToolCall
    Usage        *Usage
    FinishReason FinishReason
    Error        error
}
```

约定：`tool_call` 事件在**分片组装完成后**发出（完整 `ID`/`Name`/`Arguments`），`Agent` 直接收集即可，无需自行拼接。

### 结构化错误

```go
type ErrorKind string

const (
    ErrorKindInvalidRequest ErrorKind = "invalid_request"
    ErrorKindAuthentication ErrorKind = "authentication"
    ErrorKindRateLimit      ErrorKind = "rate_limit"
    ErrorKindTimeout        ErrorKind = "timeout"
    ErrorKindNetwork        ErrorKind = "network"
    ErrorKindServer         ErrorKind = "server"
    ErrorKindUnknown        ErrorKind = "unknown"
)

type ModelError struct {
    Kind      ErrorKind
    Message   string
    Retryable bool
    Cause     error
}

func (e *ModelError) Error() string
func (e *ModelError) Unwrap() error
```

## 实现一个模型适配器

只需实现 `Model` 接口（见 [model-openai](/modules/model-openai) 的参考实现）：

```go
type myProvider struct{ /* ... */ }

func (p *myProvider) Complete(ctx context.Context, req agentmodel.Request) (agentmodel.Response, error) {
    // 非流式：调用服务 → 组装 Message/Usage/FinishReason
}

func (p *myProvider) Stream(ctx context.Context, req agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
    // 流式：解析 SSE → 逐段发送 delta/tool_call/usage/finish/error
}
```

## 契约测试（可选）

`adapters/model-openai` 提供 `TestContract*` 契约测试（文本流/流式/Tool Call/错误映射），需要真实端点（`MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL`），无则自动 SKIP。新适配器可参考同样的断言结构。

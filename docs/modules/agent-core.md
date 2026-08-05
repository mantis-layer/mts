# agent-core — 最小 Agent Loop

`agent-core` 提供 **Model → Tool → Model** 最小循环运行时：注册工具、流式调用模型、Schema 校验参数、执行工具并把结果回写上下文，直到模型不再请求工具。它是 MTS 的执行核心，但不承担 Task 生命周期、持久化或业务逻辑。

模块路径：`github.com/mantis-layer/mts/agent-core`（仅依赖 `agent-model`）

## 设计原理

- **循环即全部**：`Agent.Run` 一次完成"模型→工具→模型…→最终消息"。需要暂停/恢复/审批/验收时，把 Agent 包装成 `Pattern` 交给 `agent-runtime`——本模块不做这些。
- **工具契约简单**：`Tool` 只有 4 个方法；参数是 `map[string]any`（由 JSON 解出），执行前经 Schema 校验。
- **可观察**：`Options.OnEvent` 输出结构化事件流（`run.start`/`model.*`/`tool.*`/`agent.*`/`run.end`），CLI 与日志可直接消费。
- **可控制**：`MaxIterations`/`MaxToolCalls` 预算、`ToolTimeout` 工具超时、context 取消向下传播、`Steering`/`ContextHook` 在每次模型调用前干预输入。
- **可替换**：模型是 `agentmodel.Model` 接口，工具是 `Tool` 接口——`New` 只依赖两者。

## 类型与接口

### `Tool` 接口

```go
type Tool interface {
    Name() string                  // 唯一工具 ID
    Description() string           // 供模型理解何时调用
    Parameters() map[string]any    // 输入 JSON Schema（object）
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}

type ToolError struct {
    Code    string `json:"code"`    // 结构化错误码，如 "file_not_found"
    Message string `json:"message"`
}
func (e *ToolError) Error() string
func NewToolError(code, message string) *ToolError
```

工具返回非 `*ToolError` 的普通错误时，Agent 会包装为 `tool.error` 事件并回写上下文。

### `Registry`

```go
type Registry struct{ /* ... */ }

func NewRegistry() *Registry
func (r *Registry) Register(t Tool) error     // 重复 Name 报错（唯一 ID）
func (r *Registry) Get(name string) (Tool, bool)
func (r *Registry) Len() int
func (r *Registry) ListTools() []Tool
func (r *Registry) Schemas() []agentmodel.ToolSchema // 供模型 tools 参数
```

### `Agent` 与 `Options`

```go
type Options struct {
    MaxIterations int          // 模型调用轮次上限（默认 10）
    MaxToolCalls  int          // 工具调用预算（默认 20）
    ToolTimeout   time.Duration // 单次工具执行超时（默认 30s）
    OnEvent       func(Event)  // 事件回调
    Steering      func(ctx context.Context, msgs []agentmodel.Message) ([]agentmodel.Message, error) // 模型调用前可修改/中止
    ContextHook   func(ctx context.Context, msgs []agentmodel.Message) []agentmodel.Message           // 输入变换
}

type Result struct {
    FinalMessage agentmodel.Message
    Usage        agentmodel.Usage
    ToolCalls    int
    Iterations   int
    Aborted      bool // context 取消时为 true
}

func New(model agentmodel.Model, registry *Registry, opts Options) *Agent
func (a *Agent) Run(ctx context.Context, input string) (Result, error)
func (a *Agent) RunWithMessages(ctx context.Context, msgs []agentmodel.Message) (Result, error)
```

### `Event` / `EventKind`

```go
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

// EventKind：run.start / model.start / model.delta / model.done /
// tool.start / tool.done / tool.error / agent.message / agent.error / run.end
```

### `ValidateJSONSchema`

```go
func ValidateJSONSchema(schema map[string]any, value any) error
```

支持子集：`type`（`string`/`number`/`integer`/`boolean`/`object`/`array`）、`required`、`properties`、`items`、`enum`、数值边界（`minimum`/`maximum`）。非法参数在工具执行前拦截（`tool.error` + 结构化错误回写）。

## 运行行为

1. 用户输入成为首条 `user` 消息；`Steering`/`ContextHook` 可在每次模型调用前变换消息。
2. `Model.Stream` 收集 `delta`/`tool_call`/`usage`/`finish`/`error`。
3. 有 `tool_call`：`Registry.Get` → `ValidateJSONSchema` → 带 `ToolTimeout` 的 `Execute` → 结果以 `RoleTool` 消息回写。
4. 无 `tool_call`：该消息即 `Result.FinalMessage`。
5. 预算耗尽或 context 取消：返回带 `Aborted` 的结果或结构化错误。

## 使用示例

```go
client, _ := modelopenai.New(modelopenai.Config{BaseURL: ..., APIKey: ..., Model: ...})

reg := agentcore.NewRegistry()
_ = reg.Register(tools.FileReader{})
_ = reg.Register(tools.Calculator{})

agent := agentcore.New(client, reg, agentcore.Options{
    MaxToolCalls: 5,
    OnEvent: func(ev agentcore.Event) {
        if ev.Kind == agentcore.EventModelDelta {
            fmt.Print(ev.Content)
        }
    },
})

res, err := agent.Run(ctx, "读取 data.json 并计算 sales 总和")
// res.FinalMessage.Content 为最终答案；res.ToolCalls / res.Usage 可审计
```

## 边界

- **不负责**：Task 生命周期、持久化、审批、Evaluator（这些在 `agent-runtime`）。
- **不负责**：任何 Provider HTTP 实现（在 `adapters/`）。
- **不负责**：密钥处理（密钥只在适配器/`agent-compose` 层解析，且要求 `${ENV}` 引用）。

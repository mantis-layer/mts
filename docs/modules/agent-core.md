# agent-core

`agent-core` 提供最小、可嵌入的 Agent Loop，不承担 Task 生命周期或持久化。

## 执行流程

```text
用户消息 → Model.Stream → Tool Call？
                         ├─ 否：返回最终消息
                         └─ 是：校验参数 → 执行工具 → 写回 Tool 消息 → 再调用模型
```

## 关键 API

| API | 作用 |
|---|---|
| `NewRegistry` / `Register` | 按唯一名称注册 `Tool` |
| `New` | 创建 Agent，并设置默认预算和超时 |
| `Run` | 用一段用户输入运行 Agent |
| `RunWithMessages` | 用已有消息列表运行 Agent |
| `Options.OnEvent` | 观察模型、工具和运行事件 |
| `Options.Steering` | 每次模型调用前修改或中止消息 |
| `Options.ContextHook` | 在调用模型前生成消息视图 |

## Tool 契约

```go
type Tool interface {
    Name() string
    Description() string
    Parameters() map[string]any
    Execute(ctx context.Context, input map[string]any) (map[string]any, error)
}
```

执行前会验证参数 JSON 与 `Parameters()` 给出的 Schema。单次工具调用默认超时 30 秒；默认上限是 10 次模型迭代和 20 次工具调用，可通过 `Options` 覆盖。

需要将这次运行变成可恢复的任务时，使用 [agent-runtime](/modules/agent-runtime)。

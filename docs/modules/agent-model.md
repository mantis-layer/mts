# agent-model

`agent-model` 定义 Provider 无关的最小模型协议。上层不需要知道具体是 OpenAI、本地模型还是另一个 Provider。

## 主要类型

| 类型 | 用途 |
|---|---|
| `Message` | `system`、`user`、`assistant`、`tool` 四种消息角色，以及 Tool Call 关联字段 |
| `ToolSchema` | 提供给模型的函数名称、说明和 JSON Schema 参数 |
| `Request` / `Response` | 非流式模型请求与响应 |
| `StreamEvent` | 文本增量、Tool Call、Usage、结束与错误事件 |
| `Model` | `Complete` 与 `Stream` 两个实现点 |
| `ModelError` | 分类 Provider、认证、限流、协议等错误 |

## 实现一个 Model

```go
type Model interface {
    Complete(context.Context, Request) (Response, error)
    Stream(context.Context, Request) (<-chan StreamEvent, error)
}
```

`agent-core` 使用 `Stream` 来收集文本、Tool Call 与 Usage。新的适配器应保证 Tool Call 的 ID、名称和 JSON 参数在事件中完整可用，并把 Provider 错误映射为 `ModelError`。

下一页：[OpenAI 兼容适配器](/modules/model-openai)。

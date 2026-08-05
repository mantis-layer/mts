# model-openai — OpenAI 兼容适配器

`adapters/model-openai` 是 `agentmodel.Model` 的**参考实现**：对接任意 OpenAI 兼容 Chat Completions 端点（官方、中转站、本地服务），支持非流式与 SSE 流式、Tool Call 分片组装、Usage/FinishReason 提取与结构化错误映射。

模块路径：`github.com/mantis-layer/mts/adapters/model-openai`（仅依赖 `agent-model`）

## 设计原理

- **纯 HTTP，不依赖厂商 SDK**：直接实现 Chat Completions 协议；核心（`agent-core`）因此对任意兼容端点可用。
- **SSE 分片组装**：流式响应中 `tool_calls` 按 `index` 分片到达，`Client` 在 `streamLoop` 中累积，待 `ID`/`Name` 完整后**一次性**发出 `StreamEventToolCall`（调用方无需拼接）。
- **错误映射**：HTTP 状态 → `agentmodel.ModelError`（401/403→`authentication`，429→`rate_limit`，5xx→`server`，超时→`timeout`，其余→`network`/`unknown`）。
- **安全**：错误体中的 `Bearer` token 与 `sk-*` 密钥在进入错误消息前脱敏（`redactSecrets`）。

## 类型与接口

```go
type Config struct {
    BaseURL string        // 端点（不含 /chat/completions）
    APIKey  string        // 认证 key（Bearer）
    Model   string        // 模型名
    HTTPClient *http.Client // 可选；默认 http.DefaultClient
}

type Client struct{ /* ... */ }
func New(cfg Config) (*Client, error)      // 校验必填字段
func (c *Client) ModelName() string        // 返回配置的模型名
func (c *Client) Complete(ctx context.Context, req agentmodel.Request) (agentmodel.Response, error)
func (c *Client) Stream(ctx context.Context, req agentmodel.Request) (<-chan agentmodel.StreamEvent, error)
```

`Client` 完整实现 `agentmodel.Model`，可直接传给 `agentcore.New` / `agentcompose.Builder`。

## 使用示例

```go
client, err := modelopenai.New(modelopenai.Config{
    BaseURL: os.Getenv("MTS_BASEURL"), // 如 https://api.example.com/v1
    APIKey:  os.Getenv("MTS_API_KEY"),
    Model:   os.Getenv("MTS_MODEL"),
})

agent := agentcore.New(client, registry, agentcore.Options{})
res, err := agent.Run(ctx, "你好")
```

## 契约测试

`TestContract*` 需要真实端点（`MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL`），无则自动 SKIP：

```bash
cd adapters/model-openai
export MTS_BASEURL=... MTS_API_KEY=... MTS_MODEL=...
go test -v -run TestContract ./...
```

覆盖：非流式文本、流式文本、Tool Call 往返、认证错误映射、`[DONE]` 收尾与 Usage。

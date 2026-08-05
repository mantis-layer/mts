# OpenAI 兼容模型适配器

模块路径：`github.com/mantis-layer/mts/adapters/model-openai`

该适配器实现 `agent-model.Model`，面向 OpenAI 兼容的 `/chat/completions` 端点，支持非流式请求与 SSE 流式请求。

## 配置

```go
client, err := modelopenai.New(modelopenai.Config{
    BaseURL: "https://your-openai-compatible-endpoint/v1",
    APIKey:  os.Getenv("MTS_API_KEY"),
    Model:   "your-model",
})
```

`BaseURL`、`APIKey` 和 `Model` 都不能为空。适配器会把 MTS Message 和 Tool Schema 映射到 Chat Completions 格式，流式合并文本与分片 Tool Call 参数，最后发出 Usage 与 Finish 事件。

## 错误处理

网络和 HTTP 响应会映射为 `agent-model.ModelError`。错误种类覆盖配置、认证、限流、Provider、协议、超时、取消和未知错误。错误文本会对 `Authorization`、`api_key`、`token`、`secret` 等敏感字段做脱敏。

契约测试可在配置 `MTS_CONTRACT_ENDPOINTS` 后运行，验证真实端点的文本、流、Tool Call 与认证错误行为。未配置时测试会跳过。

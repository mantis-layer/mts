# agent-compose

`agent-compose` 是开发者的组合层：读取 YAML/JSON Manifest，或通过 fluent Go Builder 把模型和工具装配为 `agent-core.Agent`。

## Manifest 工作流

```text
读取文件 → ParseManifest → Validate → 解析 Provider → 解析工具 → 构造 Agent
```

Manifest 要求 `api_version: v1`、非空 `name`、`model.provider` 与 `model.model`。如填写 `model.api_key`，必须使用 `${ENV_NAME}` 形式。序列化 Manifest 供调试时会将密钥替换为 `[REDACTED]`。

## Go Builder

适合由代码决定模型或工具组合的场景：

```go
agent, err := agentcompose.NewBuilder().
    Name("data-summary").
    Model(client).
    Tools(tools.FileReader{}, tools.Calculator{}).
    OnEvent(handleEvent).
    Build()
```

Builder 会拒绝缺失模型和重复工具名。Builder 当前组装的是 `agent-core.Agent`；需要 TaskRun、持久化和审批时，应将该 Agent 包装到 `agent-runtime.NewToolLoopPattern`。

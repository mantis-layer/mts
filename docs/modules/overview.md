# 模块总览

每个一级目录都是一个可独立引用的 Go Module（`go.work` 管理，`go build all` / `go test ./...` 在根目录可用）。下表给出选择入口时最实用的划分。

| 模块 | 公共 API 核心 | 适合场景 |
|---|---|---|
| [`agent-model`](./agent-model) | `Model`、`Message`、`ToolSchema`、`StreamEvent`、`Usage`、`ModelError` | 实现新的模型适配器 |
| [`agent-core`](./agent-core) | `Agent`、`Tool`、`Registry`、`Event`、`ValidateJSONSchema` | 嵌入单次 Agent 运行 |
| [`agent-runtime`](./agent-runtime) | `Runtime`、`TaskRun`、`Pattern`、`Storage`、`Evaluator`、`Artifact` | 长任务、审批、恢复、验收 |
| [`agent-plugin`](./agent-plugin) | `Plugin`、`Registry`、`Manifest`、`mcp` | 可替换扩展与跨进程工具 |
| [`agent-compose`](./agent-compose) | `Manifest`、`Builder`、`ResolveAPIKey` | 声明式组装 Agent |
| [`model-openai`](./model-openai) | `Config`、`Client`（实现 `agentmodel.Model`） | 对接 Chat Completions 服务 |
| [`tools` / `cli` / `examples`](./tools-cli-examples) | `FileReader`、`Calculator`、`mts` 命令 | 试用和参考实现 |

## 依赖关系（单向）

```text
agent-model  ←  agent-core  ←  agent-plugin / agent-compose / agent-runtime
agent-model  ←  adapters/model-openai
agent-model / agent-core  ←  tools
以上全部 ←  cli（入口）  ←  examples
```

`agent-model` 不依赖上层模块；`agent-core` 只依赖它。需要持久化生命周期时，上接 `agent-runtime`；需要把实现替换为插件或声明式配置时，再使用 `agent-plugin` 和 `agent-compose`。依赖方向由 `scripts/check-deps.sh` 在 CI 中强制（违反即失败）。

## 典型组合

| 需求 | 组合 |
|---|---|
| 流式 Tool Loop（一次运行） | `agent-core` + `adapters/model-openai` |
| 可恢复的 Task | `agent-runtime` + `SQLiteStorage` + `ToolLoopPattern` |
| 研究/报告（含证据） | `agent-runtime` + `ResearchPattern` + `tools.FileReader` |
| 人工审批工作流 | `agent-runtime` + `WorkflowPattern`（`waiting` + `SubmitHumanInput`） |
| 声明式替换模型 | `agent-compose`（Manifest + `${ENV}` 密钥）+ `agent-plugin` |
| 开箱即用 | `cli`（`mts` 命令） |

## 开发指引

- 构建：`go build all`（workspace 根）
- 测试：`go test ./...`（各 Module）；竞态：`go test -race ./...`（CI 强制）
- 依赖方向：`scripts/check-deps.sh`
- 契约测试：`adapters/model-openai`（需 `MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL`，无则 SKIP）
- 详细开发流程见 [本地开发](/development/local-development)

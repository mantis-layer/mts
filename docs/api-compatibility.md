# API 兼容说明（v0.1）

本文件描述 MTS v0.1 的公共 API 面与兼容承诺（Roadmap M6：公共 API 有示例和兼容说明）。

## 版本与兼容策略

- 采用 **语义化版本（SemVer）**：`vX.Y.Z`。
  - `X` 主版本：不兼容变更（可含破坏性 API 变更）。
  - `Y` 次版本：向后兼容的新功能。
  - `Z` 补丁：向后兼容的缺陷修复。
- **v0.x 阶段**：`0.Y.Z` 中 `Y` 可包含有限的不兼容调整（PATCH 前向兼容）；v1.0 起严格 SemVer。
- 每个 Module 独立发版（`agent-model`、`agent-core`、`agent-plugin`、`agent-compose`、`agent-runtime`、`adapters/model-openai`、`tools`、`cli`）。
- 兼容承诺：不删除公开符号；破坏性变更走主版本号并附迁移说明。

## Module 边界（依赖方向，见 `scripts/check-deps.sh`）

```
agent-model  ←  agent-core  ←  agent-plugin / agent-compose / agent-runtime
agent-model  ←  adapters/model-openai
agent-model / agent-core  ←  tools
以上全部 ←  cli（入口）  ←  examples
```

规则：**agent-model 不依赖任何内部 Module；agent-core 仅依赖 agent-model；任何 Module 不得依赖 agent-runtime**（Runtime 是上层；v0.1 的 `cli`/`examples` 尚不依赖 runtime，后续版本引入）。`scripts/check-deps.sh` 在 CI 中自动执行。

## 公共 API（v0.1）

### agent-model（模型抽象，零依赖）

| 符号 | 说明 |
|---|---|
| `Model` 接口（`Complete`/`Stream`） | 统一模型补全，非流式 + 流式（SSE） |
| `Message` / `Role` / `ToolCall` | 消息与工具调用结构 |
| `ToolSchema` / `FunctionSchema` | 工具 Schema（JSON Schema 描述） |
| `Request` / `Response` / `Usage` | 请求、响应与 token 用量 |
| `StreamEvent`（Delta/ToolCall/Usage/Finish/Error） | 流式事件 |
| `ModelError` / `ErrorKind` | 结构化模型错误（auth/rate_limit/timeout/…） |

### agent-core（最小 Agent Loop）

| 符号 | 说明 |
|---|---|
| `Agent` + `Options`（OnEvent/Steering/ContextHook） | 模型↔工具循环，事件流、取消、预算 |
| `Tool` 接口（Name/Description/Parameters/Execute） | 工具契约 |
| `Registry`（Register/Get/ListTools/Schemas） | 静态工具注册（唯一 ID） |
| `Event` / `EventKind` | Agent 事件流 |
| `Result`（FinalMessage/Usage/ToolCalls） | 运行结果 |
| `ValidateJSONSchema` | 输入 Schema 校验（子集） |

### agent-plugin（插件 + MCP）

| 符号 | 说明 |
|---|---|
| `Plugin` / `Registry` / `Manifest` | 插件注册、Manifest、版本校验、生命周期 |
| `mcp` 子包（`Client`/`ToolAdapter`） | MCP 工具适配（stdio） |

### agent-compose（声明式组合）

| 符号 | 说明 |
|---|---|
| `Manifest`（Model/Tools/Pattern） | YAML/JSON Agent 声明 |
| `Builder`（Build） | 构建 Agent |
| `ResolveAPIKey` | `${ENV}` 密钥引用（禁明文） |

### agent-runtime（Task Runtime）

| 符号 | 说明 |
|---|---|
| `Task` / `TaskRun` / `RunState` | 任务与运行实例、状态机 |
| `Runtime`（SubmitTask/Run/Cancel/SubmitHumanInput/GetRun/Events） | 运行入口 |
| `Pattern` / `StepResult`（Done/NeedHuman/Terminated/Progress/Artifacts） | Pattern 宿主 |
| `ToolLoopPattern` / `ResearchPattern` / `WorkflowPattern` | 内置三种 Pattern |
| `Storage` 接口 + `MemoryStorage` / `SQLiteStorage` | 持久化（CAS 原子写） |
| `Artifact` / `Evidence` / `Evaluator`（Schema/EvidenceCoverage） | 产出与验收 |
| `Budget` | 执行预算 |

### adapters/model-openai

| 符号 | 说明 |
|---|---|
| `Config`（BaseURL/APIKey/Model） | OpenAI 兼容端点配置 |
| `Client`（Complete/Stream） | 实现 `agentmodel.Model`（含 SSE tool_call 组装、错误映射） |

### tools / cli / examples

- `tools`：`FileReader`（JSON/CSV，密钥路径拦截）、`Calculator`（安全表达式求值）
- `cli`：`mts` 命令（`MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL` 配置、事件流输出）
- `examples/`：三个 ≤300 行示例 Agent（Tool Loop / Research / Workflow）
  - `examples/tool_loop_agent`：Tool Loop（131 行，PRD S10）
  - `examples/research_agent`：ResearchPattern → 报告 Artifact + Evidence（184 行）
  - `examples/workflow_agent`：WorkflowPattern + 人工审批（179 行）

## 示例与测试

- 构建：`go build all`；测试：`go test ./...`（各 Module）；竞态：`go test -race ./...`（CI）
- 依赖方向：`scripts/check-deps.sh`
- 契约测试：`adapters/model-openai`（需 `MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL`，无则 SKIP）
- 三场景示例：ToolLoop（`examples/tool_loop_agent`）、Research（`examples/research_agent`）、Workflow（`examples/workflow_agent`）

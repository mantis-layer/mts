# tools / cli / examples — 官方工具与可运行入口

## `tools` — 内置工具

模块路径：`github.com/mantis-layer/mts/tools`（依赖 `agent-model`、`agent-core`）

### `FileReader`

读取本地 JSON / CSV 文件并解析为结构化数据（`{"data": ..., "path": ...}`）。

```go
type FileReader struct{}
func (FileReader) Name() string // "file_reader"
func (FileReader) Description() string
func (FileReader) Parameters() map[string]any // {path: string 必填}
func (FileReader) Execute(ctx context.Context, input map[string]any) (map[string]any, error)
```

安全：`isForbiddenPath` 拦截密钥/环境配置文件（`.env*`、SSH 私钥 `id_*`、`*.pem/.key/.p12/.pfx/.ppk`，大小写不敏感），命中返回 `forbidden_path` 结构化错误。支持 `.json` 与 `.csv`（解析为 `[]map[string]string`）。

### `Calculator`

安全表达式求值器（**shunting-yard + RPN，无 eval、无外部依赖**），支持四则运算、括号、小数、一元负号。

```go
type Calculator struct{}
func (Calculator) Name() string // "calculator"
func (Calculator) Parameters() map[string]any // {expression: string 必填}
func (Calculator) Execute(...) (map[string]any, error) // {"expression":..., "result":...}
```

非法表达式返回 `invalid_expression` 结构化错误（含除零/括号不配对）。

## `cli` — `mts` 命令

模块路径：`github.com/mantis-layer/mts/cli`。最小 Agent CLI（FR-008）：通过 OpenAI 兼容端点运行带 `file_reader` + `calculator` 的 Tool Loop Agent。

```bash
cd mts/cli
go run . --task "读取 data.json，用 calculator 计算 sales 总和，输出摘要"
```

| 配置 | 说明 |
|---|---|
| `MTS_BASEURL` / `--baseurl` | OpenAI 兼容端点 |
| `MTS_API_KEY` / `--api-key` | API key（flag 默认空，`--help` 不泄露） |
| `MTS_MODEL` / `--model` | 模型名 |
| `--task` / stdin | 任务描述（`--task` 为空读 stdin） |
| `--json` | JSON Lines 事件输出 |

解析顺序：`.env.local`（自动加载）→ 环境变量 → flag（flag 优先）。`SIGINT` 优雅取消。

## `examples/` — 三个示例 Agent（v2.0：共享 Persona + Memory 抽象）

v2.0（Issue #46）三示例升级为加载 **Persona + MemoryStore + ContextBuilder** 三件套（PRD G6 / S11 / P1）：

- `tool_loop_agent`：每次模型调用前注入 LongTerm 记忆（S14/P3）。
- `research_agent`：研究产出的 Evidence 写入 LongTerm，下次 Run 可检索（跨会话恢复的写入端）。
- `workflow_agent`：HITL 输入记录进 ShortTerm 记忆（跨会话审批历史）。

三示例共享同一份构造路径（`Options{ContextBuilder, Persona, MemoryStore}`），无需修改 `agent-core` 源码即可组合（S11/P1）。记忆持久化用 `agent-runtime.VectorMemoryStore`（SQLite + sqlite-vec）；OpenAI adapter 同时实现 `Model` 与 `EmbeddingProvider`。

### `examples/tool_loop_agent` — Tool Loop（S10，v2.0 升级）

不修改 `agent-core` 源码，用 agent-core + OpenAI 兼容 adapter + 官方 tools 创建并跑通 Tool Loop Agent，并装配 Persona + Memory + ContextBuilder（PRD S10 / Issue #46）。

```bash
cd examples/tool_loop_agent
go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"

# 启用记忆持久化与注入（跨会话）：
go run . --persona-id my-agent --memory ./mem.db --embed-dim 1536 --seed-memory "用户偏好中文摘要"
```

- 输入：`examples/data.json`（示例销售数据）
- v2.0 新增 flag：`--persona-id` / `--persona-name` / `--memory` / `--embed-dim` / `--seed-memory`
- 记忆注入触发 `memory.injected` 事件（可经 `OnEvent` 观测）
- 受限网络需先 `export HTTPS_PROXY=...`

### `examples/research_agent` — Research（M5，v2.0 升级）

`agent-runtime.ResearchPattern` 多轮研究 → 报告 Artifact + Evidence → `EvidenceCoverageEvaluator` 验收，并把研究结论写入 LongTerm 记忆（下次 Run 可检索）。

```bash
cd examples/research_agent
go run . --task "读取 ../data.json，分析销售趋势并输出中文研究报告"

# 启用记忆持久化（下次运行会注入历史研究结论）：
go run . --persona-id researcher --memory ./mem.db --embed-dim 1536
```

- 需要模型配置（`MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL` 或 flag）

### `examples/workflow_agent` — Workflow（M5，无需模型，v2.0 升级）

`agent-runtime.WorkflowPattern` 多步骤工作流（读取 → 人工审批 → 汇总），展示 `WAITING_HUMAN` 暂停/恢复，并把 HITL 输入记录进 ShortTerm 记忆。

```bash
cd examples/workflow_agent
go run . --approve   # --approve 自动批准审批节点（非交互）

# 记录审批历史到记忆：
go run . --persona-id approver --memory ./mem.db --approve
```

- 步骤为纯 Go 工具调用（FileReader），**无需模型 key**，可离线运行
- 记忆用 `VectorMemoryStore(path, nil, dim)`（无 embedding provider，ShortTerm 按时间倒序检索）

### `examples/integration/` — 端到端集成验收（S11/S13/S14，Issue #46）

集成测试包，证明三示例共享抽象 + 跨会话恢复 + 记忆注入可观测，**全离线确定性**（fake Model + 词袋哈希 embedding，不联网）：

- `TestSharedAbstractionSmoke` / `TestThreeExamplesConsistentUsage` — A2/S11/P1
- `TestCrossSessionRecovery` — A3/S13/P2（Run1 写 LongTerm → Close → Run2 检索到）
- `TestCrossSessionDistinctPersonasIsolated` — 记忆按 PersonaID 隔离
- `TestMemoryInjectedEventFires` / `TestMemoryInjectedBeforeModelCall` — A4/S14/P3
- `TestToolLoopAgentWithMemory` / `TestWorkflowAgentShortTermMemory` / `TestResearchAgentProducesLongTermEvidence` — A1

```bash
cd examples/integration
go test ./...   # 离线、确定性，无需 OpenAI key
```

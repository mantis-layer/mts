# 核心概念

## Agent 与 Tool Loop

`agent-core.Agent` 运行 **Model → Tool → Model** 循环：

1. 模型以流式事件（`StreamEvent`）生成文本增量（`delta`）或工具调用（`tool_call`）。
2. 每个 `tool_call` 的 `arguments` 先经 **JSON Schema 校验**（`ValidateJSONSchema`，支持 `type`/`required`/`properties`/`items`/`enum`/数值边界子集）；非法参数产生结构化 `ToolError` 并回写上下文，由模型自行决策。
3. 工具在**带超时的上下文**（`Options.ToolTimeout`）中执行；取消（context cancel）会向下传播到模型与工具。
4. 没有 Tool Call 时，模型消息即为最终结果（`Result.FinalMessage`）。

Agent 事件通过 `Options.OnEvent` 可观察（`model.start`/`model.delta`/`model.done`/`tool.start`/`tool.done`/`tool.error`/`agent.message`/…）。

## Task 与 TaskRun

`Task` 是用户提交的任务定义（`ID`/`Name`/`Pattern`/`Input`）；`TaskRun` 是一次具体运行，生命周期状态机：

```text
created → running → completed
                  → failed
                  → cancelled
running → waiting（HITL：等待人工输入）→ running
```

`agent-runtime` 将运行状态、输入、进度（`Progress`）、Usage、结果与错误持久化到 `Storage`；每次 `Pattern.Execute` 后写入一个 **Checkpoint**（run 状态 + 事件流），因此 TaskRun 可查询、取消、在人工输入后继续，甚至跨进程重启恢复（`SQLiteStorage` 重建后从同一 `Progress` 继续）。

## Pattern

**Pattern 只决定某次运行的下一步**（`StepResult`），Runtime 负责持久化、预算、状态迁移与事件审计（FR-005）。`StepResult` 携带：

- `Done` / `NeedHuman`（进入 `waiting` 等待人工输入）/ `Terminated`（业务终止，如审批拒绝 → `failed`）
- `Progress`（Pattern 自定义进度，持久化支持幂等恢复）
- `Artifacts` / `Evidence`（本步骤产出，Runtime 原子落库）
- `Iterations` / `ToolCalls` / `Usage`（预算与成本）

内置 Pattern：

| Pattern | 行为 |
|---|---|
| `tool_loop` | 用 `agent-core` 完成一次工具循环（`ToolLoopPattern`） |
| `research` | 运行一次 Agent 研究，把输出作为报告 Artifact + Evidence（`ResearchPattern`） |
| `workflow` | 确定性步骤推进，支持 Rule Evaluator（跳过）与人工审批节点（`WorkflowPattern`） |

三个 Pattern 共享同一 Runtime Core——注册后以 `Task.Pattern` 选择（S5）。

## Artifact、Evidence 与 Evaluator

- `Artifact` 是结构化产出（`text`/`json`/`table`），`Pattern` 通过 `StepResult.Artifacts` 产出，Runtime 原子落库（`AddArtifactsEvidence` 单事务，失败不留孤立数据）。
- `Evidence` 把 Artifact 关联到来源（`Source`/`Quote`），供引用与验收。
- `Evaluator` 在任务完成路径前依次运行；任一不通过 → Run 以 `failed` 结束（并记录 `EventEvaluatorResult`）。内置 `SchemaEvaluator`（JSON 合法）与 `EvidenceCoverageEvaluator`（证据条数/覆盖率）。

## Plugin 与 Manifest

- `agent-plugin` 把模型 Provider、工具、Evaluator 或 Pattern 包装为 `Plugin` 注册到 `Registry`（版本校验、生命周期）；`mcp` 子包提供 MCP（stdio）工具适配。
- `agent-compose` 通过 YAML/JSON `Manifest` 或 Go `Builder` 组合模型与工具；密钥只允许 `${ENV}` 引用（`ResolveAPIKey`，明文报 `plaintext_api_key`）。

## 预算、取消与人工介入

- `Budget{MaxIterations, MaxToolCalls}`：超限 → `failed` + `EventBudgetExceeded`。
- 取消：context cancel（执行中）或 `Runtime.Cancel` API（触发执行中 Run 的取消函数 + CAS 迁移终态）。
- 人工介入：`waiting` 状态 + `SubmitHumanInput`，审批拒绝经 `Terminated` 以 `failed` 终止，不进 Evaluator。

## Persona、Memory 与 ContextBuilder（v2.0：数字伙伴的身份/记忆/注意力）

v2.0 把 Agent 当作"人"设计——Persona 是身份，Memory 是记忆，ContextBuilder 是注意力。三者都通过 `agentcore.Options` 装配，**无需修改 agent-core 源码**即可让 Agent 拥有跨会话连续性（S11/P1）。

- **Persona（身份，FR-010）**：`agentcontract.Persona{ID, Name, Role, SystemPrompt}`，跨会话持久存在的 Agent 身份。`Task.PersonaID` 把任务绑定到 Persona；记忆按 `PersonaID` 归档（不同 Persona 互不串扰）。
- **Memory（五层记忆，FR-011）**：`MemoryLayer` 有 `working` / `shortterm` / `longterm` / `preference` / `skill` 五层。`MemoryStore` 接口（`Save`/`Query`/`Delete`）按 `PersonaID + Layer` 归档；runtime 提供默认 `VectorMemoryStore`（SQLite + sqlite-vec 余弦相似度 Top-K）。
- **ContextBuilder（注意力，FR-012）**：在 Steering 之后、模型调用之前执行，基于 Persona + MemoryStore 检索相关记忆（默认 longterm/preference/skill 三层），按 token 预算裁剪后注入为一条 system 消息。注入触发 `memory.injected` 事件（S14/P3 可观测）。

三示例（`tool_loop_agent` / `research_agent` / `workflow_agent`）共享这套抽象：tool_loop 注入 LongTerm 记忆；research 把 Evidence 写入 LongTerm 供下次 Run 检索；workflow 把 HITL 输入记录进 ShortTerm。详见 [agent-runtime 模块](/modules/agent-runtime) 与 [tools/cli/examples](/modules/tools-cli-examples)。集成验收测试（跨会话恢复 + 记忆注入可观测）在 `examples/integration/`，全离线确定性运行。

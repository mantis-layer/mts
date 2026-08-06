# 架构与边界

Mantis Forge 的核心设计是 **小核心、强扩展、任务优先**：把模型调用、工具循环、插件、组合与任务运行时拆成可独立引用的 Go Module，依赖方向单向（抽象在下、组合在上），并由 `scripts/check-deps.sh` 在 CI 中强制。

## 依赖方向

```text
应用 / CLI / 示例
        ↓
agent-compose      agent-runtime
        ↓                 ↓
agent-plugin ───→ agent-core
        ↓                 ↓
  adapter / MCP       agent-model
                  ↗
         agent-contract
```

规则（`scripts/check-deps.sh` 严格执行）：

- `agent-model` **不依赖任何内部 Module**（Provider 无关的纯契约）。
- `agent-contract` 仅依赖 `agent-model`（统一 Usage 等基础类型）。
- `agent-core` 仅依赖 `agent-model`。
- `agent-plugin` 依赖 `agent-model`、`agent-core`。
- `agent-compose` 依赖 `agent-model`、`agent-core`、`agent-plugin`。
- `agent-runtime` 依赖 `agent-model`、`agent-core`、`agent-contract`。
- `adapters/model-openai` 仅依赖 `agent-model`。
- `tools` 依赖 `agent-model`、`agent-core`。
- **任何 Module 不得依赖 `agent-runtime`**（Runtime 是上层；v0.1 的 `cli`/`examples` 尚不依赖它，后续版本引入）。
- 依赖按**前缀匹配**：依赖 `agent-core` 即允许 `agent-core/...` 全部子包。

## 分层职责

| 层 | 负责 | 不负责 |
|---|---|---|
| `agent-model` | 消息、工具 Schema、流式事件、Usage、`Model` 接口、结构化错误 | 任何 Provider 的 HTTP 实现 |
| `agent-contract` | Task/TaskRun/RunState、Artifact/Evidence、Budget、Event、StepResult（纯数据协议） | 状态机、持久化、执行逻辑 |
| `agent-core` | 最小 Tool Loop、事件流、取消、预算、Schema 校验、Steering/Context Hook | Task 生命周期与数据库 |
| `agent-runtime` | TaskRun 状态机、Pattern Host、Checkpoint、Artifact/Evidence、Evaluator、HITL | 具体模型与业务工具 |
| `agent-plugin` | 插件契约、Registry、Manifest、生命周期、MCP Tool Adapter | Go 原生动态插件加载 |
| `agent-compose` | Manifest 校验、`${ENV}` 密钥解析、Agent 组装 | 具体模型 SDK |
| `adapters/model-openai` | OpenAI 兼容端点（非流式 + SSE 流式、错误映射） | 业务逻辑 |
| `tools` | FileReader、Calculator（官方示例工具） | Task 生命周期 |
| `cli` | `mts` 命令：配置加载、任务提交、事件流输出 | 业务逻辑 |

## 数据流：一次 Tool Loop

```text
用户任务
   │  agent-core.Agent.Run(ctx, input)
   ▼
[1] Model.Stream(messages, tools)          ── 流式 delta / tool_call / usage
   │
   ├─ 无 tool_call → FinalMessage（结束）
   ▼
[2] Registry.Get(tool) + ValidateJSONSchema(arguments)
   │  非法参数 → 结构化 ToolError 回写上下文
   ▼
[3] Tool.Execute(ctx, input)               ── 带 ToolTimeout 的上下文
   │
   ▼
[4] tool 结果（RoleTool message）回写 → 回到 [1]
```

`agent-runtime` 将上述循环放入 `TaskRun`：每一步执行后 Checkpoint 持久化，支持取消、预算、等待人工输入、产物（Artifact/Evidence）落库与 Evaluator 验收。

## 数据流：Task 生命周期

```text
SubmitTask(task)        → TaskRun{created}
Run(runID)              → running → [Pattern.Execute × N] → completed / failed / cancelled
                          (每步 UpdateRunIf + Event 落库 = Checkpoint)
Cancel(runID)           → 触发执行中 Run 的上下文取消 → cancelled
SubmitHumanInput(input) → waiting → running → 继续
恢复                     → 用同一 Storage 重建 Runtime，从 Progress 继续（幂等）
```

## 设计原则

1. **小核心，强扩展**：`agent-core` 只有 Tool Loop；长生命周期能力全部上移 `agent-runtime`，扩展能力用 Plugin/Manifest。
2. **任务优先，可验证**：产出用 `TaskResult` + `Artifact` + `Evidence` 表达，验收用 `Evaluator`（任一不通过 → failed），而非仅聊天记录。
3. **可控制**：预算（`Budget`）、取消（context + `Cancel` API）、HITL（`waiting` 状态）内建于 Runtime。
4. **可恢复**：Storage 持久化 run 状态 + 事件流；进度（`Progress`）使 Pattern 可从任意 Checkpoint 继续。
5. **Pattern 与 Runtime 职责分离**：Pattern 只回答"下一步做什么"（`StepResult`），持久化/预算/状态迁移/事件由 Runtime 统一处理（FR-005）。
6. **安全默认**：密钥仅支持 `${ENV}` 引用（禁明文）；`agent-core` 不做任何密钥处理；FileReader 拦截密钥路径；OpenAI 错误体 Bearer 脱敏。
7. **存储契约不泄漏实现**：`Storage` 接口无 SQLite 概念（FR-007）；数据保留/删除/TTL 明确延后。

## 关键架构决策（ADR 摘要）

| 决策 | 理由与实现 |
|---|---|
| `Model` 契约 = OpenAI 兼容协议（`baseurl`/`apiKey`/`model`） | 中转站天然兼容；`adapters/model-openai` 独立实现，核心不依赖厂商 SDK |
| TaskRun 状态迁移用 **CAS**（`CompareAndSetState`/`UpdateRunIf`） | 并发 Cancel/Run/HITL 不互相覆盖终态（`UPDATE ... WHERE state=?`） |
| `Pattern` 只返回 `StepResult`（Done/NeedHuman/Terminated/Progress/Artifacts/Evidence） | Runtime 统一落库、审计、预算；Pattern 可替换（ToolLoop/Research/Workflow 共享 Core） |
| SQLite 单连接 + `busy_timeout(10000)` | SQLite 单写者模型；并发写触发 `database is locked` 会静默吞掉终态 |
| 取消路径用 `context.WithoutCancel` 写 storage | 已取消的 ctx 会让 `ExecContext` 立即失败，导致 cancelled 终态写不进去 |
| 密钥 `ResolveAPIKey` 强制 `${ENV}` 形式 | 防止 Manifest 明文 key 进入仓库（Manifest 校验报 `plaintext_api_key`） |
| 依赖方向自动检查 | `check-deps.sh` 在 CI 执行，防跨层依赖回归 |
| 审批不侵入 `agent-core` | `WorkflowPattern` 只依赖 runtime 层；`agent-core` 无 HITL 概念 |

## 何时使用哪个入口

- 只需一次可流式观察的工具调用循环：直接使用 `agent-core`。
- 需要取消、等待人工输入、持久化或产物验收：使用 `agent-runtime`。
- 需要替换模型或按声明组合工具：使用 `agent-plugin` 与 `agent-compose`。
- 只需要一个开箱即用的 CLI：`mts`（`MTS_BASEURL`/`MTS_API_KEY`/`MTS_MODEL`）。

当前版本仍是早期 v0.1，公共 API 尚未冻结（见 [API 兼容说明](/api-compatibility)）。以代码和测试为当前行为的依据，产品愿景与路线图用于说明范围而非替代实现契约。

---
managed_by: scd-discovery
status: approved
version: 1
updated_at: 2026-08-05T11:10:58+08:00
approved_at: 2026-08-05T11:10:58+08:00
---

# Agent Platform v0.1 产品需求文档（PRD）

> 本文档由 `00-product-vision.md`、`01-prd-v0.1.md`、`02-validation-use-cases.md`、`03-architecture-overview.md`、`04-roadmap-v0.1.md` 五份 Draft 文档收敛而来，是 v0.1 的**产品基线**（产品级 why / users / problem / MVP scope / requirements / success metrics 的唯一权威来源）。技术设计以对应架构文档与后续 Spike 为准。

## Product vision

为 Agent 开发者提供一套 **Go 原生、模块化、插件化的 Agent Runtime 与 SDK**。开发者通过组合 Model、Pattern、Tool、Memory、Evaluator、Storage 与 Policy 构建 Agent，不必重复实现模型适配、Agent Loop、任务状态、产物、证据与验收。

v0.1 核心命题：**三个执行模式明显不同的 Agent（Tool Loop / Research / Workflow），不修改 Runtime Core，仅通过组合模块即可构建并运行**。

## Primary users

- **主用户：Agent 应用开发者（Persona A）**——用 Go API 或 Agent Manifest 组装 Agent，关注 SDK、Tool、Model、Pattern 与示例。
- **次用户：插件开发者（Persona B）**——实现可复用的 Provider / Tool / Evaluator / Storage / Pattern，依赖稳定接口与契约测试。
- **约束方（非功能驱动者）**：企业平台工程师（Persona C，关注权限/审计/持久化/私有化）、Runtime 贡献者（Persona D，关注模块边界/可测试性/维护成本）。

## User problem and current alternative

**问题**：Agent 开发在不同项目中反复实现模型适配、Tool Loop、消息流与上下文处理；会话/任务/运行状态/产物/记忆边界不清；长任务难暂停、恢复、审计、验证；插件扩展通常只覆盖 Tool，无法替换 Pattern/Memory/Evaluator/Storage；Python 框架多，缺少适合本地程序、守护进程与企业服务的 Go 原生 Runtime；Agent 常"给出了结果"却无法证明任务真正完成。

**现状替代**：自研胶水代码、各自为政的 Python 编排、把 Agent 简化为 Prompt 封装——均无法提供可验证、可恢复、可审计的执行基础设施。

## MVP goals

- G1 可独立使用的模型抽象层：统一消息、Tool Schema、流式输出、Usage、错误、Provider 元数据。
- G2 最小 Agent Core：Model ↔ Tool 循环、流式事件、取消、Steering、Context Hook。
- G3 通用 Task Runtime：Task / Run / Pattern / Budget / Artifact / Evaluator / Checkpoint / Human-in-the-loop。
- G4 正式插件扩展机制：Model Provider、Tool、Pattern、Context、Memory、Evaluator、Storage、Sandbox、Protocol Adapter。
- G5 开发者友好的组合体验：Go API 或 Agent Manifest 组装 Agent。
- G6 三条垂直切片（Tool Loop / Research / Workflow）共享核心 Runtime，验证通用性。

## Non-goals

v0.1 明确不做：自动 Agent Compiler；任意自然语言生成复杂 Graph；完整 Multi-Agent 调度平台；插件市场与商业结算；分布式大规模任务集群；自我训练/自我演化；全功能可视化编排器；通用长期人格记忆；生产级桌面 Agent 产品；复杂 Workflow DSL；所有语言的 SDK。

P1 延后项（v0.1 不交付）：gRPC / ConnectRPC 传输、Docker Sandbox、PostgreSQL Adapter、Runtime Server、Web Console、独立进程插件、多语言 Client SDK。

## Core user journeys

1. **创建 Agent**：初始化项目 → 配置 OpenAI 兼容 Provider（`baseurl` + `apiKey` + `model`）→ 注册 Tool / Pattern / Evaluator → 提交 Task → 订阅事件 → 获取 Result 与 Artifact。
2. **开发插件**：引用 Plugin API → 实现扩展接口 → 编写 Plugin Manifest → 运行契约测试 → 静态注册 → 在示例 Agent 中验证。
3. **执行长期任务**：创建 Task → Runtime 创建 TaskRun → Pattern 驱动执行 → 持久化事件与 Checkpoint → 暂停/等待人工 → 恢复执行 → Evaluator 验收 → 返回 TaskResult。

## Functional requirements

标识符沿用文档（FR-001~009，P0 级），保持稳定。

- **FR-001 Model Abstraction**：统一 Message 与 Tool Schema；支持流式文本与 Tool Call；支持 Usage 与 Finish Reason；支持取消与 Provider 错误映射；≥2 个 OpenAI 兼容端点（含中转站）通过契约测试。
- **FR-002 Agent Core**：保存 Agent State；执行 Model → Tool → Model 循环；支持多个 Tool Call；产生结构化 Event Stream；支持 Abort、Steering 与 Follow-up；支持 Context Transform Hook。
- **FR-003 Tool API**：Tool 具备唯一 ID、描述与输入输出 Schema；调用支持 Context、超时与取消；错误必须结构化；调用必须产生审计事件。
- **FR-004 Task Runtime**：创建 TaskRun；管理 Run 状态机；管理预算与取消；保存 Artifact 与 Evidence；执行 Evaluator；支持基础 Checkpoint；支持同步执行与事件订阅。
- **FR-005 Pattern Host**：至少支持 Tool Loop、Research、Workflow 三种 Pattern；Pattern 只决定下一步行为，不负责底层持久化、工具执行与预算统计。
- **FR-006 Plugin System**：静态 Go 插件注册；Plugin Manifest；类型与版本校验；插件生命周期；MCP Tool Adapter；独立进程 / WASM 插件为实验状态。
- **FR-007 Storage**：内存实现（测试）+ SQLite 实现（本地持久化）；Storage 接口不暴露 SQLite 特有概念。
- **FR-008 SDK 与 CLI**：Go Builder API；Manifest 加载；最小 CLI；事件流输出；示例 Agent 启动命令。
- **FR-009 Observability**：记录 Task/Run ID、模型调用、Tool 调用、状态转换、Token/Usage、Artifact、Evaluator 结果、错误与重试。

## Rules and failure cases

**规则**：
- 权限默认拒绝；Secret 使用引用，不进日志与 Contract；第三方插件不默认获得宿主全部权限；高风险能力允许配置人工审批（NFR-004）。
- 核心不依赖 Go 原生动态 `plugin` 包（NFR-002）；公共协议带 `apiVersion`，插件启动前做兼容性检查，扩展接口具备契约测试套件（NFR-006）。

**故障路径**（按场景）：
- Tool Loop：文件不存在、Tool 返回非法 Schema、模型重复调用相同 Tool、Tool 超时、Token 或 Tool Call 预算耗尽。
- Research：搜索工具不可用、来源内容冲突、证据覆盖不足、上下文超限、模型输出无有效引用、Runtime 中途重启。
- Workflow：非法状态跳转、未授权审批、批准消息重复发送、副作用 Tool 崩溃后重入、持久化写入失败、人工长期不响应。

**可靠性规则**：取消必须向下传播；Checkpoint 后可重启恢复；Tool 或 Provider 失败不能破坏 Run Store；重复事件写入必须可检测或幂等（NFR-003）。

## Data, permissions, and integrations

- **数据**：Task / TaskRun、Event 流、Artifact、Evidence、Checkpoint、Evaluation 持久化于 SQLite（本地）或内存（测试）。
- **数据生命周期（本次决策）**：v0.1 **不实现删除 / 保留 / TTL**，Storage 接口只含写入、读取、恢复；数据治理能力整体 Deferred，P1 出现真实清理场景后再设计。
- **权限**：按 NFR-004 安全基线；人工审批为可配置能力。
- **集成**：SQLite（本地持久化）、MCP（外部工具生态，优先兼容）；Model 接入统一走 **OpenAI 兼容协议**（配置 = `baseurl` + `apiKey` + `model name` 三要素，中转站天然兼容）。
- 核心模块不依赖具体厂商 SDK、数据库或业务 Agent。

## Success metrics

v0.1 发布前必须满足以下 10 条验收标准（量化版，标识符稳定）：

- **S1**: 三个示例 Agent 无需修改 Runtime Core 即可运行。
- **S2**: ≥2 个 OpenAI 兼容端点（含中转站，通过 `baseurl`/`apiKey`/`model` 配置）通过相同 Contract Test。
- **S3**: ≥1 个第三方风格 Tool 插件可独立注册，不修改 Core。
- **S4**: Tool Agent 完成含 ≥2 次 Tool Call 的任务，最终摘要关键数字与 Calculator 结果一致。
- **S5**: Research Agent 产出报告与证据 Artifact，通过 Evidence Coverage Evaluator。
- **S6**: Workflow Agent 可等待人工输入并从持久化状态恢复。
- **S7**: Runtime 支持预算、取消与结构化错误。
- **S8**: 核心模块无反向依赖（自动检查规则）。
- **S9**: 关键路径（模型适配 / Agent Loop / Tool 执行 / TaskRun 持久化）具备集成测试与故障测试。
- **S10**: 新开发者不改 `agent-core` 源码，在文档指引下用 ≤300 行 Go 代码创建并跑通 Tool Loop Agent。

## Assumptions and risks

**假设**：
- 具体 Provider 端点由 M1 Spike 1 选定；测试环境需要用户提供中转站 / OpenAI 的 `baseurl` 与 `apiKey`。
- "基础 Evaluator"以验证场景 A（Schema 校验）与场景 B（Evidence Coverage）的定义为准。

**风险**（沿用文档风险表，缓解措施不变）：过早抽象、Core 膨胀、插件接口过宽、Model API 抽象失真、Pattern 与 Runtime 耦合、Task 协议过大、文档先于实现。

## Open questions

- 无产品级阻塞项。
- 技术开放项（由 M1 Spike 验证，**不进产品合同**）：流式 Tool Call 统一方式、Agent Event Stream 最小事件集合、Pattern 输入输出是否采用 Decision 模型、Checkpoint 保存事件还是状态快照、Plugin Manifest 最小字段、独立进程插件 RPC、Task Contract 字段边界、Artifact 与 Evidence 是否分开建模、Runtime 内部用 Graph 还是状态机。

## Approval

- Status: Approved
- Approved version: 1
- Approved at: 2026-08-05T11:10:58+08:00
- 批准依据：scd-discovery 访谈收敛（成功标准量化、数据生命周期 defer、OpenAI 兼容 Model 契约、首条交付 = M2 Tool Loop 垂直切片）

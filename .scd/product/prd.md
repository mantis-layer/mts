---
managed_by: scd-discovery
status: approved
version: 2
updated_at: 2026-08-06T10:30:00+08:00
approved_at: 2026-08-06T10:30:00+08:00
---

# Mantis Forge v2.0 产品需求文档（PRD）

> 本文档是产品级 why / users / problem / MVP scope / requirements / success metrics 的唯一权威来源。
>
> **版本演进**：v1（2026-08-05）建立通用 Agent Runtime 基线，三垂直切片（Tool Loop / Research / Workflow）已实现并验收。v2（2026-08-06）立意升级——从"通用 Agent Runtime"演进为"数字伙伴 Runtime"，引入 Persona（身份）、Memory（五层记忆）、Context Builder（注意力）三个架构图已规划但 v0.1 未实现的组件，并完成品牌与模块的一次性重命名（Mantis Forge / `mantis-*`）。技术设计以架构文档与 Spike 为准。

## Product vision

**Mantis Forge — 搭建属于你的数字伙伴。**
A modular runtime for building capable, persistent and evolving AI partners.

把 Agent 当作"人"来设计：**Persona 是身份，Memory 是记忆，Context Builder 是注意力**。仍是模块化、可组合的 Runtime——不是叠加"伙伴产品层"，而是用"人"作为设计罗盘，补全架构图（`03-architecture-overview.md` §3.3/§3.7/§4.1）里 v0.1 缺失的组件。开发者通过组合 Model、Pattern、Tool、Memory、Persona、Evaluator、Storage 与 Policy，构造拥有身份、记忆与持续性的数字个体。

v2.0 核心命题：**三个示例 Agent（Tool Loop / Research / Workflow）共享 Persona + Memory 抽象，跨会话保持身份与记忆连续性，且无需修改 mantis-core。**

## Primary users

- **主用户：Agent 应用开发者（Persona A）**——用 Go API 或 Manifest 组装具备身份与记忆的数字伙伴，关注 SDK、Persona、Memory、Tool、Model、Pattern 与示例。
- **次用户：插件开发者（Persona B）**——实现可复用的 Provider / Tool / Evaluator / Storage / Memory / Pattern，依赖稳定接口与契约测试。
- **约束方（非功能驱动者）**：企业平台工程师（Persona C，关注权限/审计/持久化/私有化）、Runtime 贡献者（Persona D，关注模块边界/可测试性/维护成本）。

## User problem and current alternative

**问题**：v0.1 的 Agent 是出色的 stateless 执行器，但**毫无"伙伴"属性**——无身份（每次 Run 像临时雇佣）、无记忆（跨会话一切清零）、无注意力管理（消息全量传，长任务爆 token）。架构图早已画出 Memory（§3.7）、Context Builder（§3.3）、Persona/Skill 等插件类型（§4.1），但 v0.1 都未实现。结果是：Agent 用得越久，越像金鱼——每次对话都从零开始，无法积累关系、偏好与专长。

**现状替代**：把 Persona/记忆塞进业务应用的 Prompt 字符串、自研 cookie-cutting 上下文管理、把记忆硬编码进 Tool——均无法提供可组合、可验证、可恢复的"伙伴"基础设施。

## MVP goals

- **G1 模型抽象层（v0.1 已实现，v2.0 扩展）**：统一 Message、Tool Schema、流式、Usage、错误；**v2.0 新增 EmbeddingProvider 接口**，支持向量检索所需的 embedding 生成。
- **G2 最小 Agent Core（v0.1 已实现，v2.0 扩展）**：Model ↔ Tool 循环、事件、取消、Steering；**v2.0 新增 ContextBuilder**，每次模型调用注入相关记忆。
- **G3 通用 Task Runtime（v0.1 已实现）**：Task / Run / Pattern / Budget / Artifact / Evaluator / Checkpoint / HITL 保留不动。
- **G4 Persona（身份，v2.0 新增）**：一等实体，跨会话持久存在，Task 通过 PersonaID 关联。
- **G5 Memory（五层记忆，v2.0 新增）**：Working / ShortTerm / LongTerm / Preference / Skill，按 Persona 归档，默认带向量检索的实现 + 可替换插件接口。
- **G6 三垂直切片验证（v0.1 已实现，v2.0 升级）**：三示例共享 Persona + Memory 抽象，验证伙伴连续性。
- **G7 品牌/模块重命名（v2.0 新增）**：一次性迁移到 Mantis Forge / `mantis-*`。

## Non-goals

v2.0 明确不做（延后到 v2.1+）：
- Skill 作为独立插件类型（首批作为 Memory 的 Skill 层，不单独建插件接口）。
- Growth / 学习轨迹 / 自我演化（"成长"维度暂不实现）。
- Plan / Graph、独立 Scheduler、Audit 持久化、调用级 Retry/Recovery（Runtime 内部未决项，沿用 v0.1 决策）。
- Protocol / Transport 层（gRPC / WebSocket / CBOR / Session / AuthN/AuthZ / Flow Control）。
- PostgreSQL、Object Store、Docker/K8s/Remote Sandbox、Observability（Metrics/Tracing/Alerting）。
- 完整 Multi-Agent、插件市场、分布式集群、可视化编排器。
- 沿用 v0.1 non-goals：自动 Agent Compiler、复杂 Workflow DSL、所有语言 SDK。

## Core user journeys

1. **定义伙伴身份**：创建 Persona（name + role + system prompt）→ 持久化 → 跨会话复用同一身份。
2. **伙伴记忆**：Run 产出（消息/证据/偏好）自动写入对应 Memory 层 → 跨 Run 按 Persona 归档 → 下次 Run 通过 ContextBuilder 检索注入。
3. **组装伙伴**：初始化项目 → 配置 Provider → 绑定 Persona + MemoryStore → 注册 Tool/Pattern → 提交 Task → 伙伴带着身份和记忆执行 → 返回 Result + 更新记忆。
4. **执行长期任务**（沿用 v0.1）：Task → TaskRun → Pattern → Checkpoint → HITL → 恢复 → Evaluator → TaskResult。

## Functional requirements

v0.1 已实现的 FR-001~009 保持稳定（基线不变），v2.0 新增 FR-010~014：

- **FR-001 Model Abstraction**（v0.1 基线）：统一 Message/Tool Schema/流/Usage/错误；≥2 个 OpenAI 兼容端点通过契约测试。
- **FR-002 Agent Core**（v0.1 基线）：Model↔Tool 循环、事件、Abort、Steering、ContextHook。
- **FR-003 Tool API**（v0.1 基线）：Schema、超时、结构化错误、审计事件。
- **FR-004 Task Runtime**（v0.1 基线）：TaskRun 状态机、Budget、Artifact、Evidence、Evaluator、Checkpoint、HITL。
- **FR-005 Pattern Host**（v0.1 基线）：Tool Loop / Research / Workflow。
- **FR-006 Plugin System**（v0.1 基线）：静态注册、Manifest、版本校验、生命周期、MCP。
- **FR-007 Storage**（v0.1 基线）：Memory + SQLite 实现，接口不暴露 SQLite 概念。
- **FR-008 SDK 与 CLI**（v0.1 基线，v2.0 更名）：Builder API、Manifest 加载、`mantis` CLI、事件流。
- **FR-009 Observability**（v0.1 基线）：Task/Run/Model/Tool/状态/Usage/Artifact/Evaluator 事件。
- **FR-010 Persona（身份）**【v2.0 新增】：`mantis-contract.Persona{ID,Name,Role,SystemPrompt,CreatedAt,UpdatedAt}`；Task 新增 PersonaID；Runtime.SubmitTask 加载关联 Persona；Storage 新增 SavePersona/GetPersona/ListPersonas；Persona 持久化跨会话复用。
- **FR-011 Memory（五层记忆）**【v2.0 新增】：`mantis-contract` 定义 MemoryStore 接口（Save/Query/Delete by persona+layer）、Memory 类型、MemoryLayer enum（Working/ShortTerm/LongTerm/Preference/Skill）；`mantis-runtime` 提供默认 VectorMemoryStore 实现（SQLite + sqlite-vec 纯 Go 向量 + 余弦相似度 Top-K）；MemoryStore 为接口，可被插件替换。
- **FR-012 Context Builder（注意力/工作记忆）**【v2.0 新增】：`mantis-contract.ContextBuilder` 接口（输入 Persona + MemoryStore + 当前消息，返回注入相关记忆后的消息视图）；`mantis-core.Options` 新增 ContextBuilder 字段，在 Steering 之后、模型调用之前执行；原 ContextHook 保留为轻量钩子（向后兼容）。
- **FR-013 EmbeddingProvider**【v2.0 新增】：`mantis-model.EmbeddingProvider` 接口（`Embed(ctx, []string) ([][]float32, error)`），与 Model 并列；OpenAI adapter 同时实现两者。
- **FR-014 一次性重命名**【v2.0 新增】：github path `mantis-labs/mantis-forge`；模块 `agent-*`→`mantis-*`（model/core/runtime/contract/plugin/compose）；CLI `mts`→`mantis`；所有 import path/package/go.work/go.mod/check-deps.sh/CI/README/docs/示例同步更新。

## Rules and failure cases

**规则**（沿用 v0.1 + 新增）：
- 权限默认拒绝；Secret 使用引用，不进日志与 Contract（NFR-004）。
- 核心不依赖 Go 原生动态 plugin；公共协议带 apiVersion；扩展接口具备契约测试（NFR-006）。
- **【v2.0】Persona 是身份唯一锚点；Memory 按 PersonaID 归档；Preference/Skill 作为 Memory 层而非 Persona 字段**（职责分离：Persona 管"是谁"，Memory 管"会什么/偏好什么"）。
- **【v2.0】依赖方向**：`mantis-core → mantis-contract → mantis-model`；Persona/MemoryStore/ContextBuilder 接口上移到 contract，使 core 能在不依赖 runtime 的前提下感知它们。
- **【v2.0】纯 Go 约束**：向量依赖必须纯 Go 无 CGO（sqlite-vec）；embedding 通过 EmbeddingProvider 接口由 Provider 实现，核心不绑定具体 embedding 服务。

**故障路径**（v0.1 沿用 + v2.0 新增）：
- v0.1 沿用：Tool/Research/Workflow 各场景的失败与恢复路径保持不变。
- **【v2.0】Persona**：PersonaID 不存在、SystemPrompt 为空、跨会话 Persona 被并发修改。
- **【v2.0】Memory**：MemoryStore 写入失败、向量检索无结果（graceful 降级为空注入）、embedding 生成失败、跨会话记忆 schema 迁移。
- **【v2.0】ContextBuilder**：检索超时不应阻塞主循环（降级为不注入）。

**可靠性**：取消向下传播；Checkpoint 可恢复；Tool/Provider 失败不破坏 Run Store；**【v2.0】Memory 写入失败不破坏 Run 终态**（记忆是增强，不是硬性约束）。

## Data, permissions, and integrations

- **数据**（v0.1 沿用）：Task / TaskRun / Event / Artifact / Evidence / Checkpoint / Evaluation。
- **【v2.0 新增数据】**：Persona（身份记录）、Memory（五层记忆记录，含 embedding 向量）、PersonaID 作为 Memory 与 Task 的归属键。
- **数据生命周期**：v0.1 不实现删除/TTL（沿用）；**v2.0 同样 defer Memory 清理**，等真实治理场景出现再设计。
- **权限**：NFR-004 基线不变。
- **集成**：SQLite（含 sqlite-vec 向量）、MCP；Model 走 OpenAI 兼容协议；**embedding 走 OpenAI 兼容 embedding 端点**。
- 核心模块不依赖具体厂商 SDK、数据库或业务 Agent。

## Success metrics

v0.1 的 S1~S10 保持为已达成基线，v2.0 新增 S11~S16：

- **S11**: 三示例共享 Persona + Memory 抽象，无需修改 mantis-core 即可组合（验证通用性）。
- **S12**: VectorMemoryStore 通过存储契约测试（与 MemoryStorage 对等的 Save/Query/Delete 契约）。
- **S13**: 跨会话恢复——Run1 写入 LongTerm 记忆 → 进程退出 → Run2 基于同一 PersonaID 检索到（端到端集成测试）。
- **S14**: ContextBuilder 在每次模型调用注入记忆，可通过事件流观测（MemoryInjected 事件）。
- **S15**: OpenAI adapter 同时通过 Model 与 EmbeddingProvider 契约测试。
- **S16**: `check-deps.sh` 在新模块名（mantis-*）下通过，依赖方向无回归。

## Assumptions and risks

**假设**：
- sqlite-vec 为纯 Go 实现，不引入 CGO，保持 v0.1 跨平台编译友好。
- OpenAI 兼容端点同时提供 chat 与 embedding 能力（或开发者配置独立 embedding 端点）。
- 五层记忆的 Working 层不做 embedding（它是当前上下文，规则检索即可），只 LongTerm/Preference/Skill 做 embedding，控制成本。

**风险**（v2.0 特有，已由决策者知情并接受）：
- **R1 五层全做 vs 先垂直**：与 v0.1 "不做纸面架构"教训有张力；首批工程量大。缓解——每层都用三示例真实驱动，不做无使用场景的层。
- **R2 内置向量检索**：引入 sqlite-vec 纯 Go 依赖（无 CGO，但仍是新依赖）；embedding 调用依赖 Provider。缓解——MemoryStore 是接口，向量实现可整体替换。
- **R3 五层 embedding 成本**：写入与检索成本高。缓解——Working 层不做 embedding；按需检索而非全量注入。
- **R4 一次性大重命名**：PR 大、所有 import 改动。缓解——单独 commit、CI 即时验证、go.work 一次性切换。

## Open questions

- 无产品级阻塞项。
- 技术开放项（由实现 Spike 验证，不进产品合同）：
  - sqlite-vec 的具体 DSN 与 schema 形态（纯 Go driver 集成方式）。
  - 五层记忆各自的 Metadata schema 精确字段（由三示例驱动后冻结）。
  - ContextBuilder 检索的 token 预算裁剪策略（注入多少记忆不爆上下文）。
  - Memory 写入时机（每步 vs Run 结束 vs 异步）。

## Approval

- Status: Approved
- Approved version: 2
- Approved at: 2026-08-06T10:30:00+08:00
- 批准依据：scd-discovery v2.0 访谈收敛（Persona 为一等实体、五层记忆全做、内置向量默认实现 + 插件可换、ContextBuilder 进 mantis-core.Options、EmbeddingProvider 独立接口、全重命名一次到位、五层 Memory 与 Persona 接口上移 mantis-contract 保持依赖方向自洽）。
- v1 基线（2026-08-05）：通用 Agent Runtime，三垂直切片已实现验收。

# Agent Platform v0.1 路线图与实施计划

> 原则：按垂直切片推进，不先横向实现所有模块。

## 1. 总体阶段

```text
M0 产品与架构收敛
→ M1 关键技术 Spike
→ M2 最小 Agent 垂直切片
→ M3 插件与组合
→ M4 Task Runtime
→ M5 Research / Workflow 验证
→ M6 v0.1 稳定与发布
```

---

# M0：产品与架构收敛

## 目标

冻结第一阶段要解决的问题、验证场景和模块边界。

## 交付物

- Product Vision
- Lean PRD
- Validation Use Cases
- Architecture Overview
- Module Boundary v0.1
- 初始 ADR

## 关键 ADR

- ADR-001：使用 Go
- ADR-002：使用 Monorepo 多 Go Module
- ADR-003：Agent Core 与 Task Runtime 分离
- ADR-004：不使用 Go 原生动态 plugin 作为主插件机制
- ADR-005：插件首先支持静态注册和 MCP
- ADR-006：所有公共协议带版本

## 退出条件

- MVP 和非目标明确
- 三个验证场景得到认可
- 五个核心 Module 的职责无明显重叠
- 依赖方向被写成自动检查规则

---

# M1：关键技术 Spike

## Spike 1：Model Streaming

实现两个 Provider 的最小 Adapter，验证：

- 文本流
- Tool Call 流
- Usage
- Finish Reason
- Provider 错误
- 取消

### 产出

- Model Event 草案
- Provider 差异记录
- Contract Test 初版

## Spike 2：Agent Core Loop

实现：

```text
Message → Model → Tool → Model → Final
```

验证：

- 多 Tool Call
- Tool 失败
- Abort
- Steering
- Event Stream
- Context Hook

### 产出

- Agent Core API 草案
- Agent Event 草案

## Spike 3：插件方式

验证：

- 静态注册
- MCP Tool Adapter
- 实验性独立进程插件

### 产出

- Plugin API 草案
- Plugin Manifest 最小字段
- 插件安全风险清单

## Spike 4：Checkpoint

实现一个最小 Run：

```text
执行一半 → 保存 → 进程退出 → 重启恢复
```

### 产出

- Run Store 抽象
- Checkpoint 方案对比
- SQLite 原型

## 退出条件

- 最危险的接口经过真实代码验证
- 架构文档根据结果更新
- 暂不冻结未经验证的复杂接口

---

# M2：最小 Agent 垂直切片

## 目标

跑通 Tool Loop Agent。

## 实现范围

```text
agent-model
agent-core
Tool API
静态 Tool Registry
一个模型 Adapter
File / Calculator Tool
CLI
In-memory Store
基础事件和 Usage
```

## 演示流程

```text
用户提交任务
→ Agent 调用读取工具
→ Agent 调用计算工具
→ Agent 输出结构化结果
```

## 测试

- Agent Core 单元测试
- Model Adapter 契约测试
- Tool 契约测试
- Tool 失败和超时
- 中途取消
- Budget 耗尽

## 退出条件

- 核心 Agent Loop 稳定
- Agent Core 可被独立使用
- 不依赖 Task Runtime 也能运行

---

# M3：插件与组合

## 目标

开发者不修改 Core 即可替换 Provider 和 Tool。

## 实现范围

- Plugin API
- Plugin Manifest
- Static Registry
- Version / Type Validation
- Agent Manifest
- Go Builder API
- MCP Tool Adapter
- 插件契约测试

## 示例

```text
同一个 Tool Agent
├── Provider A
├── Provider B
└── MCP Tool
```

## 退出条件

- 替换 Provider 不改 Agent Core
- 新 Tool 插件可独立开发
- 无效 Manifest 能给出明确错误
- 插件权限需求可声明

---

# M4：Task Runtime

## 目标

把一次 Agent 交互提升为可管理、可验证的 Task。

## 实现范围

- Task Contract 最小版
- TaskRun
- Run State
- Pattern Host
- Budget / Policy
- Artifact / Evidence
- Evaluator
- Event / Audit
- SQLite Store
- Checkpoint
- TaskResult

## 首批 Pattern

- Tool Loop Pattern
- 简单 Plan-and-Execute / Research Pattern
- Workflow 状态机 Pattern

## 退出条件

- Task 与 Agent Session 边界清晰
- Task 可以取消、失败、部分成功
- Run 可恢复
- Result 能说明验收是否通过
- Task Contract 不包含具体执行计划

---

# M5：Research 与 Workflow 验证

## Research Agent

实现：

- 搜索与读取
- Evidence Artifact
- Context Builder
- Citation / Evidence Evaluator
- Checkpoint

## Workflow Agent

实现：

- Rule Evaluator
- WAITING_HUMAN
- Approval API / CLI
- 恢复
- 幂等保护
- 审计

## 退出条件

- 三个场景共享 Runtime Core
- Pattern 可替换
- Human Approval 不侵入 Agent Core
- Evidence 和 Artifact 模型自然
- 未使用的抽象被删除或降级为实验性

---

# M6：v0.1 稳定与发布

## 稳定工作

- API Review
- Module Boundary Review
- Race Test
- Fault Injection
- Benchmark
- 安全检查
- 文档补齐
- 示例清理
- 版本和兼容策略

## 发布内容

```text
agent-model v0.1
agent-core v0.1
agent-contract v0.1
agent-runtime v0.1
agent-plugin v0.1
三个示例 Agent
开发者文档
契约测试套件
```

## v0.1 Definition of Done

- 核心 Module 可独立导入
- 三个验证场景通过
- 两个 Provider 通过契约测试
- Tool 插件可扩展
- Task 支持预算、取消和结果验收
- Research 支持证据 Artifact
- Workflow 支持人工暂停与恢复
- SQLite 恢复测试通过
- 核心依赖规则自动检查
- 公共 API 有示例和兼容说明

---

# 2. 推荐开发顺序

不要按模块横向完成，而按以下切片：

```text
Slice 1
Model + Core + Tool + CLI

Slice 2
Plugin + Agent Manifest + Provider 替换

Slice 3
Task + Run + Artifact + Evaluator

Slice 4
SQLite + Checkpoint + Human Approval

Slice 5
Research + Workflow + 故障测试
```

每个 Slice 都必须可以运行、演示和测试。

---

# 3. Issue / Epic 建议

## Epic A：Foundation

- Monorepo 和 go.work
- Module 骨架
- CI
- lint / test / race
- 文档目录
- 依赖边界检查

## Epic B：Model

- Message Schema
- Tool Schema
- Stream Event
- Provider A
- Provider B
- Contract Tests

## Epic C：Agent Core

- State
- Agent Loop
- Tool Dispatcher
- Event Stream
- Abort
- Steering
- Context Hook

## Epic D：Plugin

- Plugin API
- Registry
- Manifest
- Validator
- MCP Adapter
- Contract Tests

## Epic E：Runtime

- Task / Run
- Pattern
- Budget
- Artifact
- Evaluator
- Checkpoint
- Result

## Epic F：Persistence

- Memory Store
- SQLite
- Migration
- Recovery Test

## Epic G：Examples

- Tool Agent
- Research Agent
- Workflow Agent

## Epic H：Developer Experience

- SDK Builder
- CLI
- Debug Trace
- Tutorials
- API Reference

---

# 4. 风险控制

## 风险：文档无限增长

措施：

- 每份文档必须回答一个决策问题
- 未被实现验证的规范标记为 Draft
- 同一概念只保留一个权威文档

## 风险：架构不断扩大

措施：

- v0.1 只以三个验证场景作为需求来源
- 新功能必须说明对应哪个场景
- 无法进入垂直切片的能力不进入 Core

## 风险：接口过早稳定

措施：

- Spike 前不承诺兼容
- v0.x 允许有控制的破坏性调整
- 先发布实验 Package，再升级为公共 API

## 风险：核心与插件耦合

措施：

- Core 只认识接口和描述符
- 具体实现全部放 Adapter / Plugin
- 使用契约测试，不在 Core 中写插件特判

## 风险：Task Runtime 变成万能工作流引擎

措施：

- 只实现 Agent 所需的执行语义
- 不在 v0.1 实现复杂 DSL
- 确定性业务流程可通过 Adapter 接成熟工作流系统

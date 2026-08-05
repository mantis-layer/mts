# Agent Platform Lean PRD v0.1

> 状态：Draft  
> 目标发布：v0.1  
> 产品类型：开发者基础设施 / Agent Runtime / SDK

## 1. 背景

构建一个可长期运行、可扩展、可验证的 Agent，通常不仅需要模型和工具调用，还需要任务状态、上下文、产物、权限、持久化、错误恢复和评估。

现有项目经常在每个 Agent 应用中重复实现这些能力，导致：

- 实现重复
- 模块边界不清
- 扩展方式不统一
- 任务难以恢复
- 第三方插件难以安全接入
- Agent 输出无法可靠验收

本项目希望提供一套 Go 原生、模块化、插件化的 Agent Runtime 与 SDK。

## 2. 产品目标

### G1：提供可独立使用的模型抽象层

统一消息、Tool Schema、流式输出、Usage、错误和 Provider 元数据。

### G2：提供最小 Agent Core

实现模型与工具之间的基本循环，并支持流式事件、取消、Steering 和 Context Hook。

### G3：提供通用 Task Runtime

管理 Task、Run、Pattern、Budget、Artifact、Evaluator、Checkpoint 和 Human-in-the-loop。

### G4：提供正式插件扩展机制

支持第三方扩展 Model Provider、Tool、Pattern、Context、Memory、Evaluator、Storage、Sandbox 和 Protocol Adapter。

### G5：提供开发者友好的组合体验

开发者可以使用 Go API 或 Agent Manifest 组装 Agent。

### G6：使用三条垂直切片验证通用性

Tool Loop、Research 和 Workflow 示例必须共享核心 Runtime。

## 3. 非目标

v0.1 不包括：

- 自动 Agent Compiler
- 任意自然语言自动生成复杂 Graph
- 完整 Multi-Agent 调度平台
- 插件市场和商业结算
- 分布式大规模任务集群
- 自我训练或自我演化
- 全功能可视化编排器
- 通用长期人格记忆
- 生产级桌面 Agent 产品

## 4. 用户角色

### Persona A：Agent 应用开发者

希望快速构建一个具体 Agent，主要关心 SDK、Tool、Model、Pattern 和示例。

### Persona B：插件开发者

希望开发可复用的 Provider、Tool、Evaluator、Storage 或 Pattern，并获得稳定接口和契约测试。

### Persona C：企业平台工程师

关心权限、审计、持久化、私有化、版本兼容和故障恢复。

### Persona D：Runtime 贡献者

关心模块边界、可测试性、性能、兼容性和维护成本。

## 5. 核心用户旅程

### 5.1 创建一个 Agent

```text
初始化项目
→ 选择模型 Provider
→ 注册 Tool / Pattern / Evaluator
→ 配置 Agent
→ 提交 Task
→ 订阅事件
→ 获取 Result 和 Artifact
```

### 5.2 开发一个插件

```text
引用 Plugin API
→ 实现扩展接口
→ 编写 Plugin Manifest
→ 运行契约测试
→ 注册或加载插件
→ 在示例 Agent 中验证
```

### 5.3 执行一个长期任务

```text
创建 Task
→ Runtime 创建 TaskRun
→ Pattern 驱动执行
→ 保存事件和 Checkpoint
→ 暂停或等待人工
→ 恢复执行
→ Evaluator 验收
→ 返回 TaskResult
```

## 6. 功能需求

### P0：v0.1 必须具备

#### FR-001 Model Abstraction

- 统一 Message 和 Tool Schema
- 支持流式文本与 Tool Call
- 支持 Usage 和 Finish Reason
- 支持取消和 Provider 错误映射
- 至少实现两个 Provider Adapter

#### FR-002 Agent Core

- 保存 Agent State
- 执行 Model → Tool → Model 循环
- 支持一个或多个 Tool Call
- 产生结构化 Event Stream
- 支持 Abort、Steering 和 Follow-up
- 支持 Context Transform Hook

#### FR-003 Tool API

- Tool 具备唯一 ID、描述和输入输出 Schema
- Tool 调用支持 Context、超时和取消
- Tool 错误必须结构化
- Tool 调用必须产生审计事件

#### FR-004 Task Runtime

- 创建 TaskRun
- 管理 Run 状态机
- 管理预算和取消
- 保存 Artifact 和 Evidence
- 执行 Evaluator
- 支持基础 Checkpoint
- 支持同步执行和事件订阅

#### FR-005 Pattern Host

至少支持：

- Tool Loop Pattern
- Research Pattern 示例
- Workflow Pattern 示例

Pattern 决定下一步行为，但不负责底层持久化、工具执行和预算统计。

#### FR-006 Plugin System

v0.1 支持：

- 静态 Go 插件注册
- Plugin Manifest
- 类型和版本校验
- 插件生命周期
- MCP Tool Adapter

独立进程和 WASM 插件可以先进入实验状态。

#### FR-007 Storage

- 内存实现用于测试
- SQLite 实现用于本地持久化
- Storage 接口不暴露 SQLite 特有概念

#### FR-008 SDK 与 CLI

- Go Builder API
- Manifest 加载
- 最小 CLI
- 事件流输出
- 示例 Agent 启动命令

#### FR-009 Observability

记录：

- Task 和 Run ID
- 模型调用
- Tool 调用
- 状态转换
- Token / Usage
- Artifact
- Evaluator 结果
- 错误与重试

### P1：v0.1 后可增加

- 独立进程插件
- gRPC / ConnectRPC 传输
- Docker Sandbox
- PostgreSQL Adapter
- Runtime Server
- Web Console
- Remote Agent Adapter
- 多语言 Client SDK

## 7. 非功能需求

### NFR-001 模块边界

- `agent-model` 不依赖 Agent Runtime。
- `agent-core` 不依赖 Task Runtime。
- `agent-contract` 不依赖任何执行模块。
- Core 不依赖具体 Provider、数据库或业务 Tool。

### NFR-002 可移植性

- 核心支持 Linux、macOS。
- Windows 兼容性作为重要设计约束。
- 核心不依赖 Go 原生动态 `plugin` 包。

### NFR-003 可靠性

- 取消必须向下传播。
- Checkpoint 后可重启恢复。
- Tool 或 Provider 失败不能破坏 Run Store。
- 重复事件写入必须可检测或幂等。

### NFR-004 安全

- 权限默认拒绝。
- Secret 使用引用，不进入日志和 Contract。
- 第三方插件不默认获得宿主全部权限。
- 高风险能力允许配置人工审批。

### NFR-005 可观测性

- 每次模型和 Tool 调用可追踪。
- Run 的状态变化可重放。
- Usage 可按 Task、Run、Provider 和 Tool 聚合。

### NFR-006 兼容性

- 公共协议带 `apiVersion`。
- 插件启动前执行兼容性检查。
- 所有扩展接口具备 Contract Test Suite。

## 8. 技术约束

- 主要语言：Go
- 代码组织：Monorepo
- Module 组织：多个 Go Module
- 配置：YAML / JSON
- 内部类型：Go Struct
- Schema：JSON Schema
- 本地持久化：SQLite
- 外部工具生态：优先兼容 MCP

## 9. v0.1 范围

### 核心 Module

```text
agent-model
agent-core
agent-contract
agent-runtime
agent-plugin
```

### 官方 Adapter

```text
model-openai 或兼容 Provider
第二个模型 Provider
tool-http
tool-calculator
storage-memory
storage-sqlite
mcp-tool-adapter
```

### 示例应用

```text
tool-agent
research-agent
workflow-agent
```

## 10. 验收标准

v0.1 发布前必须满足：

1. 三个示例 Agent 无需修改 Runtime Core。
2. 至少两个模型 Provider 通过相同 Contract Test。
3. 至少一个第三方风格 Tool 插件可独立注册。
4. Tool Agent 能完成包含两次以上 Tool Call 的任务。
5. Research Agent 能产生报告与证据 Artifact，并通过基础 Evaluator。
6. Workflow Agent 能等待人工输入并从持久化状态恢复。
7. Runtime 支持预算、取消和结构化错误。
8. 核心模块没有反向依赖。
9. 关键路径具备集成测试和故障测试。
10. 新开发者可依据文档在较少代码内创建一个 Agent。

## 11. 主要风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 过早抽象 | API 难用、开发变慢 | 用三个验证场景约束设计 |
| Core 膨胀 | 模块无法独立使用 | 为每个 Module 写“不负责什么” |
| 插件接口过宽 | 安全和兼容性差 | 分类型接口、最小权限、契约测试 |
| Model API 抽象失真 | Provider 特性丢失 | 先实现两个 Provider Spike |
| Pattern 与 Runtime 耦合 | 无法替换执行模式 | Pattern 只返回 Decision，不管理基础设施 |
| Task 协议过大 | 难以演进 | v0.1 仅覆盖必要字段 |
| 文档先于实现 | 形成纸面架构 | 所有公共 API 必须经过 Spike |

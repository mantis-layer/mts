# Agent Platform 产品愿景

> 文档状态：Draft v0.1  
> 工作名称：Agent Platform（后续可替换为正式项目名）  
> 核心技术方向：Go、模块化 Runtime、插件化、Monorepo 多 Go Module

## 1. 产品定义

Agent Platform 是一套面向 Agent 开发者的模块化运行时与 SDK。

它为不同类型的 Agent 应用提供通用基础设施，包括：

- 统一模型调用与流式事件
- 最小 Agent Loop
- 工具注册、调用与错误处理
- 多步骤任务运行与状态管理
- 上下文构建与压缩
- 产物、证据与验收
- 插件发现、加载、权限与隔离
- 持久化、检查点、暂停与恢复
- 预算、策略、审计和可观测性

开发者不需要重复实现这些基础设施，而是通过组合模型、Pattern、Tool、Memory、Evaluator、Storage 和 Policy，构建研究助手、工作流助手、数据分析助手、企业助手或其他自定义 Agent。

## 2. 为什么要做

当前 Agent 开发存在几个重复且难以工程化的问题：

1. 不同项目反复实现模型适配、Tool Loop、消息流和上下文处理。
2. Agent 的执行模式经常与业务逻辑、工具和 Prompt 耦合。
3. 会话、任务、运行状态、产物和记忆之间缺少清晰边界。
4. 长任务难以暂停、恢复、审计和验证。
5. 插件扩展通常只覆盖 Tool，无法替换 Pattern、Memory、Evaluator、Storage 等组件。
6. 许多框架偏 Python 应用编排，缺少适合本地程序、守护进程和企业服务的 Go 原生 Runtime。
7. Agent 常常“给出了结果”，但无法证明任务真正完成。

Agent Platform 的目标不是再做一个 Prompt 框架，而是提供一套可靠的 Agent 执行基础设施。

## 3. 目标用户

### 3.1 Agent 产品开发者

希望快速搭建 Agent 产品，但不想重复实现底层运行机制。

### 3.2 企业 AI 平台团队

需要统一管理模型、工具、权限、任务、审计和私有化部署。

### 3.3 垂直行业解决方案团队

需要把专业知识、业务工具、执行模式和验证规则封装成可复用模块。

### 3.4 Go 工程师

希望使用静态类型、单二进制部署、并发能力和较低运行开销构建 Agent 服务。

## 4. 核心价值主张

### 4.1 小核心，强扩展

核心模块保持精简，具体 Provider、Tool、Pattern、Memory、Evaluator 和 Storage 通过插件接入。

### 4.2 一个 Runtime，多种 Agent 模式

Tool Loop、Plan-and-Execute、Research、Workflow、Review、Human Approval 等执行方式共享同一套任务、状态、产物和策略基础设施。

### 4.3 任务优先，而不是聊天记录优先

系统明确区分：

- Agent 会话
- Task
- TaskRun
- Execution
- Artifact
- Evidence
- Evaluation
- Memory

### 4.4 可验证、可控制、可恢复

Agent 不只是输出文本，还要能够：

- 遵守权限和预算
- 记录关键事件
- 产生结构化产物
- 提供证据
- 通过验收标准
- 在失败后重试或恢复

### 4.5 面向本地与企业部署

核心能力不依赖特定云厂商，可运行于本地 CLI、桌面应用、服务端、容器和私有化环境。

## 5. 产品原则

1. **Runtime-first**：优先解决运行、状态、恢复和验证，而不是堆 Prompt 抽象。
2. **Core stays small**：Agent Core 只负责最小 Agent Loop。
3. **Composition over inheritance**：Agent 通过模块组合，而不是复杂继承体系。
4. **Capabilities over implementations**：任务声明需要什么能力，Resolver 再选择具体实现。
5. **Explicit contracts**：跨模块的数据使用明确的结构和版本。
6. **Safe by default**：最小权限、显式授权、危险动作可审批。
7. **Observable by default**：模型调用、工具调用、状态转换和产物可追踪。
8. **Progressive complexity**：单 Agent 是默认方式，确有需要时再使用 Graph 或 Multi-Agent。
9. **Use cases constrain abstractions**：所有抽象必须经过真实验证场景。
10. **Go-native, protocol-friendly**：核心使用 Go，同时允许通过 MCP、RPC 等协议连接其他语言生态。

## 6. 第一阶段产品形态

第一阶段采用：

```text
一个 GitHub Organization
└── 一个核心 Monorepo
    ├── 多个独立 Go Module
    ├── 官方 Adapter 与插件
    ├── 示例 Agent
    ├── CLI / Runtime Server
    └── 文档与契约测试
```

首批核心 Module：

```text
agent-model
agent-core
agent-contract
agent-runtime
agent-plugin
```

## 7. 第一阶段目标

第一阶段要证明：

> 三个执行模式明显不同的 Agent，可以不修改 Runtime Core，仅通过组合模块完成构建和运行。

验证场景：

1. Tool Loop Agent
2. Research Agent
3. Workflow Agent

## 8. 第一阶段非目标

暂不实现：

- 自动从自然语言生成任意 Agent
- 完整 Multi-Agent 平台
- 插件 Marketplace
- 大规模分布式调度
- 自我学习和自我进化
- 通用知识图谱记忆
- 复杂 Workflow DSL
- 全功能桌面或 Web 管理台
- 所有语言的 SDK

## 9. 长期方向

长期可以扩展为：

```text
Agent Runtime
+ Plugin Ecosystem
+ Control Plane
+ Registry
+ Multi-language SDK
+ Application Templates
```

但长期愿景不能反向污染 v0.1 的核心边界。

## 10. 成功定义

v0.1 成功至少满足：

- 核心 Module 可以独立引用。
- Runtime Core 不依赖具体模型 SDK、数据库或业务 Agent。
- 至少两个模型 Provider 通过统一契约测试。
- 第三方可以实现并注册 Tool 插件。
- 三个验证 Agent 共用同一套 Runtime。
- Task 支持取消、预算限制、结构化事件和结果验收。
- Workflow 示例支持暂停、持久化和人工恢复。
- 关键模块具备单元、契约、集成和故障测试。

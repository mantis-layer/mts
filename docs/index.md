---
layout: home

hero:
  name: Mantis Forge
  text: 搭建属于你的数字伙伴
  tagline: 面向 Go 开发者的模块化 Agent Runtime 与 SDK——可验证、可控制、可恢复。
  actions:
    - theme: brand
      text: 开始使用
      link: /guide/getting-started
    - theme: alt
      text: 查看 GitHub
      link: https://github.com/mantis-layer/mts

features:
  - title: 小核心，强扩展
    details: 将模型、工具、插件、执行模式与任务运行时拆成可独立引用的 Go 模块。
  - title: 任务优先
    details: 用 Task、TaskRun、Artifact、Evidence 与 Evaluator 表达可追踪的工作，而不仅是聊天记录。
  - title: 安全且可恢复
    details: 内置预算、取消、状态机、持久化 Checkpoint 和人工介入的基础能力。
---

## 从一个 Tool Loop 开始

Mantis Forge 的最小闭环是：模型决定调用工具，工具结果回写上下文，模型输出最终答案。需要更长生命周期时，再把这个闭环放入 `agent-runtime` 的 TaskRun 中。

阅读 [开始使用](/guide/getting-started) 运行内置 CLI，或进入 [模块总览](/modules/overview) 选择要集成的能力。

> **品牌与代码命名**：产品品牌为 Mantis Forge；代码模块名 `agent-*`、CLI `mts`、环境变量 `MTS_*` 为内部稳定标识（刻意决策，类比 Kubernetes：仓库与代码命名稳定，产品品牌独立）。

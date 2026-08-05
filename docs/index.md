---
layout: home

hero:
  name: MTS
  text: 模块化 Agent Runtime 与 SDK
  tagline: 用 Go 组合可验证、可控制、可恢复的 Agent。
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

MTS 的最小闭环是：模型决定调用工具，工具结果回写上下文，模型输出最终答案。需要更长生命周期时，再把这个闭环放入 `agent-runtime` 的 TaskRun 中。

阅读 [开始使用](/guide/getting-started) 运行内置 CLI，或进入 [模块总览](/modules/overview) 选择要集成的能力。

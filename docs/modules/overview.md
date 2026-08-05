# 模块总览

每个一级目录都是一个可独立引用的 Go module。下表给出选择入口时最实用的划分。

| 模块 | 入口能力 | 适合场景 |
|---|---|---|
| [`agent-model`](./agent-model) | Provider 无关模型契约 | 实现新的模型适配器 |
| [`agent-core`](./agent-core) | 最小 Agent Tool Loop | 嵌入单次 Agent 运行 |
| [`agent-runtime`](./agent-runtime) | TaskRun 与持久化执行 | 长任务、审批、恢复、验收 |
| [`agent-plugin`](./agent-plugin) | Plugin Registry 与 MCP | 可替换扩展与跨进程工具 |
| [`agent-compose`](./agent-compose) | Manifest / Builder 组合 | 声明式组装 Agent |
| [`model-openai`](./model-openai) | OpenAI 兼容客户端 | 对接 Chat Completions 服务 |
| [`tools-cli-examples`](./tools-cli-examples) | 内置工具与可运行入口 | 试用和参考实现 |

`agent-model` 不依赖上层模块；`agent-core` 只依赖它。需要持久化生命周期时，上接 `agent-runtime`；需要把实现替换为插件或声明式配置时，再使用 `agent-plugin` 和 `agent-compose`。

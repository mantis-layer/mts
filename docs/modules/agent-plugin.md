# agent-plugin

`agent-plugin` 是类型化扩展点与 Registry。当前使用静态 Go 注册，不依赖 Go 原生动态 `plugin` 包。

## 支持的插件类型

| 类型 | 接口职责 |
|---|---|
| `model_provider` | 提供一个 `agent-model.Model` |
| `tool` | 提供一个或多个 `agent-core.Tool` |
| `evaluator` | 提供任务验收器 |
| `pattern` | 提供任务执行模式 |

所有插件必须提供 `PluginManifest`，其中的名称、SemVer 版本、`api_version: v1` 和类型都会被校验。Registry 负责初始化、名称冲突检查、类型匹配以及逆序关闭已注册插件。

## MCP Tool Adapter

`agent-plugin/mcp` 通过 stdio JSON-RPC 连接 MCP Server：初始化客户端、读取工具清单，然后把每个远程工具转换为 `agent-core.Tool`。Adapter 会将 MCP 内容打包成字符串结果，并把服务器声明的错误反馈给 Agent。

MCP 连接用于跨进程/跨语言工具；它不是运行时内置工具的替代品。对可信且同进程的 Go 工具，直接实现 `agent-core.Tool` 更简单。

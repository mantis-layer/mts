# 内置工具、CLI 与示例

## tools

`tools` 是两个参考 Tool 的集合：

- `Calculator`：接收 `expression`，支持基本四则运算、括号与一元负号，返回计算结果。
- `FileReader`：接收 `path`，读取 JSON 或 CSV 文件，返回解析后的内容。它拒绝绝对路径、路径遍历、隐藏目录与非 JSON/CSV 文件。

这些工具用于演示 Tool Schema 与执行边界；生产系统应按自己的数据访问与权限模型实现工具。

## CLI

模块路径：`github.com/mantis-layer/mts/cli`

CLI 在启动时注册 `file_reader` 与 `calculator`，创建 OpenAI 兼容客户端，并将 `agent-core` 事件打印到终端。正常输出包含最终文本、Usage 与工具/模型轮次统计；收到 `SIGINT` 或 `SIGTERM` 时会取消运行并以退出码 130 退出。

## examples

`examples/tool_loop_agent` 是一个最小的可运行示例，与 CLI 使用相同的核心组合。根目录 `examples/data.json` 提供可供 FileReader 读取的示例数据。

```bash
cd examples/tool_loop_agent
go run .
```

运行前仍需要设置 `MTS_BASEURL`、`MTS_API_KEY` 与 `MTS_MODEL`。

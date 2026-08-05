# 开始使用

MTS 是一个 Go 多模块仓库。最短路径是通过内置 CLI，使用 OpenAI 兼容端点运行一个带 `file_reader` 和 `calculator` 的 Tool Loop Agent。

## 前置条件

- Go `1.25.4`（仓库 `go.work` 与各模块的 `go.mod` 当前声明版本）
- 一个可访问的 OpenAI 兼容 Chat Completions 端点
- Node.js 18+（仅在编写或预览本站时需要）

## 运行 CLI

```bash
git clone https://github.com/mantis-layer/mts.git
cd mts/cli

export MTS_BASEURL="https://your-openai-compatible-endpoint/v1"
export MTS_API_KEY="your-api-key"
export MTS_MODEL="your-model"

go run . --task "读取 ../examples/data.json，用 calculator 计算 sales 字段的总和，然后输出一段中文摘要"
```

CLI 也接受 `--baseurl`、`--api-key`、`--model` 和 `--task` 参数；传入 `--json` 时会输出 JSON Lines 事件。

> 密钥只应从环境变量、旗标或本机 `.env.local` 提供。不要将密钥写入 Agent Manifest 或提交到仓库。

## 第一个嵌入式 Agent

在 Go 程序中，创建模型、注册工具，再运行 `agent-core`：

```go
client, err := modelopenai.New(modelopenai.Config{
    BaseURL: os.Getenv("MTS_BASEURL"),
    APIKey:  os.Getenv("MTS_API_KEY"),
    Model:   os.Getenv("MTS_MODEL"),
})
if err != nil {
    return err
}

registry := agentcore.NewRegistry()
_ = registry.Register(tools.FileReader{})
_ = registry.Register(tools.Calculator{})

agent := agentcore.New(client, registry, agentcore.Options{})
result, err := agent.Run(ctx, "读取 data.json，并计算 sales 总和")
```

下一步：理解 [核心概念](/guide/concepts)，或直接查看 [`agent-core`](/modules/agent-core) 的职责与边界。

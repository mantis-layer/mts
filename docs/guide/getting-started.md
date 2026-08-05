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

### CLI 配置优先级

`mts` 命令按以下顺序解析配置（`cli/main.go`）：

1. 先自动加载仓库根的 `.env.local`（若存在；已写入 `.gitignore`，不会提交）
2. 再读取环境变量 `MTS_BASEURL` / `MTS_API_KEY` / `MTS_MODEL`
3. 最后是 flag `--baseurl` / `--api-key` / `--model`（显式传入优先于 env；flag 默认值为空，避免 `--help` 泄露密钥）

`--task` 为空时从 **stdin** 读取任务；`--json` 输出 JSON Lines 事件流；进程响应 `SIGINT`（Ctrl-C）优雅取消。受限网络需要代理时，先 `export HTTPS_PROXY=...`。

> 密钥只应从环境变量、旗标或本机 `.env.local` 提供。不要将密钥写入 Agent Manifest（`agent-compose` 会校验并拒绝明文 `api_key`）或提交到仓库。

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

## 进入 Task Runtime

需要取消、人工审批、持久化或产物验收时，把 Agent 包装成 Pattern 交给 `agent-runtime`：

```go
rt, _ := agentruntime.NewRuntime(agentruntime.NewMemoryStorage(), agentruntime.Budget{MaxToolCalls: 10})
_ = rt.RegisterPattern(agentruntime.NewToolLoopPattern(agent)) // 复用上面的 agent

run, _ := rt.SubmitTask(ctx, &agentruntime.Task{ID: "task-1", Pattern: "tool_loop", Input: "读取 data.json 计算 sales 总和"})
final, _ := rt.Run(ctx, run.ID)
// final.State == completed；final.Result.Summary 为最终输出
```

参考 [模块总览](/modules/overview) 选择适合你的组合。

## 下一步

- [核心概念](/guide/concepts) — TaskRun、Pattern、Artifact/Evidence/Evaluator
- [架构与边界](/guide/architecture) — 分层与设计决策
- [API 兼容说明](/api-compatibility) — 公共 API 与版本策略

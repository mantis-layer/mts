# Tool Loop Agent 示例

本目录包含 Tool Loop 垂直切片的示例输入与 S10 验证示例。

## 目录

- `data.json` — 示例销售数据
- `tool_loop_agent/` — **PRD S10 验证示例**：131 行 Go 代码（≤300 行），不改 `agent-core`
  源码，用 `agent-compose` Builder + OpenAI 兼容 adapter + 官方 tools 创建并跑通 Tool Loop Agent。
- `README.md` — 本文档

## 本地运行

```bash
cd examples/tool_loop_agent
go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"
# 配置：MTS_BASEURL / MTS_API_KEY / MTS_MODEL（或 --baseurl/--api-key/--model）
# 自动加载仓库根 .env.local；需要代理时先 export HTTPS_PROXY=...
```

## 交付检查

- **S8（依赖方向自动检查）**：`scripts/check-deps.sh` 验证核心模块无反向依赖
  （agent-model ← agent-core ← agent-plugin ← agent-compose，adapters 只依赖 model）。
- **S10（≤300 行创建 Agent）**：本示例 `main.go` 共 131 行（`wc -l`），不改
  `agent-core` 源码，真实端点跑通（file_reader → calculator → 摘要，总和 1525）。

## 契约测试（真实端点）

```bash
cd adapters/model-openai
export MTS_BASEURL=... MTS_API_KEY=... MTS_MODEL=...
go test -v -run TestContract ./...
```

> 注意：部分受限网络（沙箱/代理 allowlist）可能无法直连中转站，此时契约测试会
> SKIP 并注明"网络不可达"，这是环境限制而非失败。

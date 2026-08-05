# Tool Loop Agent 示例

本目录包含 v0.1 发布的三类示例 Agent（Tool Loop / Research / Workflow），
每例均 ≤300 行、不改 `agent-core` 源码、可独立构建运行。

## 目录

- `data.json` — 示例销售数据
- `tool_loop_agent/` — **Tool Loop 示例**（131 行）：`agent-compose` Builder + OpenAI
  兼容 adapter + 官方 tools 创建并跑通 Tool Loop Agent（PRD S10 验证）。
- `research_agent/` — **Research 示例**（184 行）：`agent-runtime` ResearchPattern 多轮
  研究 → 报告 Artifact + Evidence → `EvidenceCoverageEvaluator` 验收。
- `workflow_agent/` — **Workflow 示例**（179 行）：`agent-runtime` WorkflowPattern 多步骤
  工作流（读取 → 人工审批 → 汇总），展示 WAITING_HUMAN 暂停与恢复。
- `mcp/python_echo_server.py` — MCP 跨语言联调用零依赖 Python MCP stdio server。
- `README.md` — 本文档

## 本地运行

```bash
# Tool Loop（需模型配置）
cd examples/tool_loop_agent
go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"

# Research（需模型配置）
cd examples/research_agent
go run . --task "读取 ../data.json，分析销售趋势并输出中文研究报告"

# Workflow（无需模型；--approve 自动批准人工审批）
cd examples/workflow_agent
go run . --approve

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

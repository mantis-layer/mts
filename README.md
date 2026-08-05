<p align="center">
  <img src="assets/mts-mantis-hero.png" alt="MTS 的复古机械螳螂主视觉" width="100%" />
</p>

<h1 align="center">MTS</h1>

<p align="center">
  <strong>面向 Go 开发者的模块化 Agent Runtime 与 SDK。</strong><br />
  小核心，强扩展；任务优先，可验证、可控制、可恢复。
</p>

<p align="center">
  <a href="https://mantis-layer.github.io/mts/">在线文档</a> ·
  <a href="#快速开始">快速开始</a> ·
  <a href="#模块地图">模块地图</a> ·
  <a href="https://github.com/mantis-layer/mts/issues">Issues</a>
</p>

<p align="center">
  <a href="https://github.com/mantis-layer/mts/actions/workflows/ci.yml"><img src="https://github.com/mantis-layer/mts/actions/workflows/ci.yml/badge.svg" alt="CI 状态" /></a>
  <a href="https://github.com/mantis-layer/mts/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-0f766e.svg" alt="MIT License" /></a>
  <img src="https://img.shields.io/badge/Go-1.25-00ADD8.svg" alt="Go 1.25" />
  <a href="https://mantis-layer.github.io/mts/"><img src="https://img.shields.io/badge/Docs-Online-2563eb.svg" alt="在线文档" /></a>
</p>

---

MTS 提供构建 Agent 应用所需的执行基础设施：统一的模型协议、最小 Model → Tool 循环、可组合插件、Task 生命周期与持久化、Artifact / Evidence / Evaluator，以及面向真实运行的预算、取消和人工介入能力。

它不是另一个 Prompt 框架。MTS 让你用可替换的 Model、Tool、Pattern、Storage 和 Policy，组合出研究助手、确定性工作流、数据 Agent 或企业私有化 Agent。

> **v0.1**：公共 API 仍在收敛中。请以源码、测试和[在线文档](https://mantis-layer.github.io/mts/)为当前行为依据。

## 快速开始

MTS 当前提供一个基于 OpenAI 兼容端点的最小 CLI。它会注册 `file_reader` 与 `calculator`，运行一个 Tool Loop Agent。

```bash
git clone https://github.com/mantis-layer/mts.git
cd mts/cli

export MTS_BASEURL="https://your-openai-compatible-endpoint/v1"
export MTS_API_KEY="your-api-key"
export MTS_MODEL="your-model"

go run . --task "读取 ../examples/data.json，用 calculator 计算 sales 字段的总和，然后输出一段中文摘要"
```

也可以把配置放在本机 `.env.local`，或通过 `--baseurl`、`--api-key`、`--model` 传入。使用 `--json` 可得到 JSON Lines 事件流。

## 为什么是 MTS

| 关注点 | MTS 的边界 |
|---|---|
| **最小 Agent Loop** | `agent-core` 只解决 Model → Tool → Model、事件、取消、Steering 与 Context Hook。 |
| **任务而非聊天记录** | `agent-runtime` 用 Task / TaskRun / Artifact / Evidence / Evaluator 表达可管理的工作。 |
| **可替换能力** | Provider、Tool、Pattern 与 Evaluator 可以通过明确契约注册或组合。 |
| **恢复与控制** | 状态机、预算、取消、SQLite Checkpoint 与 Human-in-the-loop 面向真实的中断路径。 |
| **Go 原生，协议友好** | 多 Go Module 独立引用；MCP Tool Adapter 连接跨进程、跨语言工具。 |

## 模块地图

```text
应用 / CLI / 示例
        ↓
agent-compose      agent-runtime
        ↓                 ↓
agent-plugin ───→ agent-core
        ↓                 ↓
  adapter / MCP       agent-model
```

| 模块 | 说明 |
|---|---|
| [`agent-model`](agent-model) | 与 Provider 无关的 Message、Tool Schema、流、Usage 与 Model 接口。 |
| [`agent-core`](agent-core) | 可嵌入的最小 Agent Loop、Tool Registry 与结构化事件。 |
| [`agent-runtime`](agent-runtime) | TaskRun 状态机、Pattern Host、存储、预算、Checkpoint、Artifact、Evidence、Evaluator。 |
| [`agent-plugin`](agent-plugin) | 类型化插件契约、生命周期 Registry 与 MCP Tool Adapter。 |
| [`agent-compose`](agent-compose) | YAML/JSON Manifest、密钥引用校验和 fluent Go Builder。 |
| [`adapters/model-openai`](adapters/model-openai) | OpenAI 兼容 Chat Completions 客户端与流式 Tool Call 处理。 |
| [`tools`](tools) | `calculator` 与受路径限制的 `file_reader` 参考实现。 |
| [`cli`](cli) / [`examples`](examples) | 可直接运行的 Tool Loop 入口与垂直切片示例。 |

## 运行时能力

- **模型与工具**：统一流式模型协议、Tool JSON Schema 校验、超时与结构化错误。
- **执行可观测性**：模型、工具、状态迁移、Checkpoint 与 Evaluator 结果均可作为事件读取。
- **任务控制**：Run 具备 `created`、`running`、`waiting`、`completed`、`failed`、`cancelled` 状态，取消与状态更新使用原子保护。
- **持久化**：提供内存实现用于测试，以及 SQLite 存储用于本地持久化和恢复。
- **组合与扩展**：可用代码或 Manifest 组合 Agent；当前插件类型包括模型 Provider、Tool、Evaluator 和 Pattern。

详细行为、边界与 API 示例，请从[在线文档](https://mantis-layer.github.io/mts/)开始。

## 开发

仓库使用 Go workspace，根目录不是单一 Go module。修改后可执行：

```bash
go test ./agent-model ./agent-core ./agent-runtime ./agent-plugin/... \
  ./agent-compose ./adapters/model-openai ./tools ./cli ./examples/tool_loop_agent

scripts/check-deps.sh
```

预览或编写项目文档：

```bash
npm ci
npm run docs:dev
```

完整开发环境、配置、测试说明见[开发者文档](https://mantis-layer.github.io/mts/development/local-development)。

## 文档与路线图

- [在线文档站](https://mantis-layer.github.io/mts/) — 开始使用、概念、模块说明、配置和测试。
- [产品愿景](00-product-vision.md) — 项目要解决的问题与原则。
- [验证场景](02-validation-use-cases.md) — Tool Loop、Research 与 Workflow 的验证目标。
- [总体架构](03-architecture-overview.md) — 分层、依赖方向和扩展平面。
- [v0.1 路线图](04-roadmap-v0.1.md) — 当前阶段与非目标。

## 螳螂

MTS 的品牌形象是一只机械螳螂：观察、等待、精确行动。它对应的不是“更复杂的 Agent”，而是对每次模型调用、工具副作用和任务状态保持清晰边界与控制。

## 参与贡献

欢迎通过 [Issues](https://github.com/mantis-layer/mts/issues) 讨论问题、用例和设计取舍。提交改动前请保持模块依赖方向、补充相应测试，并避免将密钥、真实端点凭证或构建产物提交到仓库。

## License

[MIT](LICENSE) © mantis-layer

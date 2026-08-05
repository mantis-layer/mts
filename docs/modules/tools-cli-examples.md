# tools / cli / examples — 官方工具与可运行入口

## `tools` — 内置工具

模块路径：`github.com/mantis-layer/mts/tools`（依赖 `agent-model`、`agent-core`）

### `FileReader`

读取本地 JSON / CSV 文件并解析为结构化数据（`{"data": ..., "path": ...}`）。

```go
type FileReader struct{}
func (FileReader) Name() string // "file_reader"
func (FileReader) Description() string
func (FileReader) Parameters() map[string]any // {path: string 必填}
func (FileReader) Execute(ctx context.Context, input map[string]any) (map[string]any, error)
```

安全：`isForbiddenPath` 拦截密钥/环境配置文件（`.env*`、SSH 私钥 `id_*`、`*.pem/.key/.p12/.pfx/.ppk`，大小写不敏感），命中返回 `forbidden_path` 结构化错误。支持 `.json` 与 `.csv`（解析为 `[]map[string]string`）。

### `Calculator`

安全表达式求值器（**shunting-yard + RPN，无 eval、无外部依赖**），支持四则运算、括号、小数、一元负号。

```go
type Calculator struct{}
func (Calculator) Name() string // "calculator"
func (Calculator) Parameters() map[string]any // {expression: string 必填}
func (Calculator) Execute(...) (map[string]any, error) // {"expression":..., "result":...}
```

非法表达式返回 `invalid_expression` 结构化错误（含除零/括号不配对）。

## `cli` — `mts` 命令

模块路径：`github.com/mantis-layer/mts/cli`。最小 Agent CLI（FR-008）：通过 OpenAI 兼容端点运行带 `file_reader` + `calculator` 的 Tool Loop Agent。

```bash
cd mts/cli
go run . --task "读取 data.json，用 calculator 计算 sales 总和，输出摘要"
```

| 配置 | 说明 |
|---|---|
| `MTS_BASEURL` / `--baseurl` | OpenAI 兼容端点 |
| `MTS_API_KEY` / `--api-key` | API key（flag 默认空，`--help` 不泄露） |
| `MTS_MODEL` / `--model` | 模型名 |
| `--task` / stdin | 任务描述（`--task` 为空读 stdin） |
| `--json` | JSON Lines 事件输出 |

解析顺序：`.env.local`（自动加载）→ 环境变量 → flag（flag 优先）。`SIGINT` 优雅取消。

## `examples/tool_loop_agent` — S10 示例

`examples/tool_loop_agent/`：**131 行 Go**（≤300 行），不修改 `agent-core` 源码，用 `agent-compose.Builder` + OpenAI 兼容 adapter + 官方 tools 创建并跑通 Tool Loop Agent（PRD S10 验证示例）。

```bash
cd examples/tool_loop_agent
go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"
```

- 输入：`examples/data.json`（示例销售数据）
- 交付检查：S8 依赖方向脚本、S10 行数与真实端点运行（file_reader → calculator → 摘要）均由 CI/手动验证
- 受限网络需先 `export HTTPS_PROXY=...`

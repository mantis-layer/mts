# agent-plugin — 插件系统与 MCP

`agent-plugin` 提供插件的**声明、注册、生命周期与扩展点**，并内置 **MCP（Model Context Protocol，stdio）工具适配器**。它让"替换模型、追加工具、扩展 Evaluator/Pattern"以可审计的方式完成，而不修改核心代码。

模块路径：`github.com/mantis-layer/mts/agent-plugin`（依赖 `agent-model`、`agent-core`）

## 设计原理

- **Manifest 先行**：每个插件带 `PluginManifest`（`name`/`version`/`apiVersion`/`type`/`permissions`/`capabilities`），注册时先 `ValidateManifest`（版本兼容、类型合法、版本格式）——不合法直接拒绝，不残留状态。
- **扩展点即 Go 接口**：四种 `PluginType` 对应四种可选接口（`ToolPlugin`/`ModelProviderPlugin`/`EvaluatorPlugin`/`PatternPlugin`），注册时按声明类型做类型断言；类型不符 → 明确错误。
- **注册原子性**：`Registry.Register` 锁外完成 `Init`/扩展提取，锁内查重与写入；任一环节失败回滚已写入内容（无半注册）。
- **MCP 走 stdio JSON-RPC**：`mcp` 子包把外部进程暴露的工具包成 `agentcore.Tool`，跨进程扩展工具而不改核心。

## 类型与接口

### `Plugin` 基础契约

```go
type Plugin interface {
    Manifest() PluginManifest // 插件元数据
    Init(ctx context.Context) error // 注册时调用；失败则不注册
    Close() error                   // 退出时释放资源
}
```

### 扩展点

```go
type ToolPlugin interface {
    Plugin
    Tools() []agentcore.Tool
}
type ModelProviderPlugin interface {
    Plugin
    Model() (agentmodel.Model, error)
}
// EvaluatorPlugin / PatternPlugin：v0.1 仅契约标记，Evaluator/Pattern 类型由
// agent-runtime 消费（Evaluators() []any / Patterns() []any）
```

### `PluginManifest` 与校验

```go
type PluginType string // model_provider | tool | evaluator | pattern

type PluginManifest struct {
    Name         string
    Version      string
    APIVersion   string // 必须等于 CurrentAPIVersion
    Type         PluginType
    Description  string
    Permissions  []string // 权限声明（供授权/审计查询）
    Capabilities []string
}

func ValidateManifest(m PluginManifest) error // missing_name / missing_api_version / unsupported_api_version / unknown_type / 版本格式
```

### `Registry`

```go
type Registry struct{ /* ... */ }

func NewRegistry() *Registry
func Register(ctx context.Context, p Plugin) error // 校验 + Init + 查重 + 原子写入
// 另提供：注册工具/模型的扁平视图（tools、models map），供 Compose/CLI 消费
```

### `mcp` 子包（MCP 工具适配）

```go
type Client struct{ /* ... */ }
func NewClient(ctx context.Context, command string, args ...string) (*Client, error) // 启动子进程（stdio JSON-RPC）

type ToolInfo struct{ /* 外部工具元数据 */ }
type ToolAdapter struct{ /* ... */ }
func NewToolAdapter(client *Client, info ToolInfo, name string) *ToolAdapter // 实现 agentcore.Tool
```

## 使用示例

```go
reg := agentplugin.NewRegistry()
_ = reg.Register(ctx, &myToolPlugin{})          // 实现 ToolPlugin
_ = reg.Register(ctx, &myModelPlugin{})         // 实现 ModelProviderPlugin

// MCP 工具
client, _ := mcp.NewClient(ctx, "python", "mcp_server.py")
adapter := mcp.NewToolAdapter(client, mcp.ToolInfo{Name: "fetch"}, "fetch")
_ = coreRegistry.Register(adapter)              // 作为普通 Tool 使用
```

## 边界

- 不做 **Go 原生动态插件加载**（`plugin` 包）；跨进程用 MCP stdio。
- Manifest 校验只保证声明合法；**权限执行**由上层（应用/CLI）消费 `Permissions` 实施（v0.1 声明模型）。

# agent-compose — 声明式组合与组装

`agent-compose` 提供两种组装 Agent 的方式：**声明式 Manifest**（YAML/JSON）与 **Go fluent Builder**。二者产出相同的 `agent-core.Agent`，把"换模型、换工具、组合扩展"变成配置而非代码。

模块路径：`github.com/mantis-layer/mts/agent-compose`（依赖 `agent-model`、`agent-core`、`agent-plugin`）

## 设计原理

- **模型由调用方注入**：`Compose` 接受 `ModelFactory`（按 `ModelSpec` 返回 `agentmodel.Model`），`agent-compose` 自身不反向依赖任何具体 adapter——保持依赖方向单向。
- **密钥安全**：`ModelSpec.APIKey` 只允许 `${ENV_NAME}` 引用；`Validate` 发现明文直接拒绝（`plaintext_api_key`），防止密钥进仓库。
- **Manifest 与 Builder 等价**：`Builder.Build()` 与 `Compose()` 走同一注册/查重逻辑（工具名冲突 → `duplicate_tool`）。
- **插件衔接**：模型可来自 plugins registry 中的 `model_provider` 插件，工具可从插件或内置 `tools` 解析。

## 类型与接口

### `AgentManifest` / `ModelSpec`

```go
type ModelSpec struct {
    Provider string `yaml:"provider"`            // model_provider 插件名
    BaseURL  string `yaml:"base_url,omitempty"`  // OpenAI 兼容端点（无插件时内联）
    APIKey   string `yaml:"api_key,omitempty"`   // 仅 ${ENV} 引用
    Model    string `yaml:"model"`
}

type AgentManifest struct {
    APIVersion  string    `yaml:"api_version"` // 必须 == CurrentAPIVersion
    Kind        string    `yaml:"kind"`        // "agent"
    Name        string    `yaml:"name"`
    Model       ModelSpec `yaml:"model"`
    Tools       []string  `yaml:"tools"`
    Pattern     string    `yaml:"pattern,omitempty"`
    Permissions []string  `yaml:"permissions,omitempty"`
}

type ManifestError struct{ Code, Message string }
func (e *ManifestError) Error() string
```

### 加载与校验

```go
func LoadManifest(path string) (*AgentManifest, error) // 按扩展名读 YAML/JSON
func ParseManifest(data []byte) (*AgentManifest, error)
func (m *AgentManifest) Validate() error
// 校验：apiVersion / kind / name / model.provider / model.model /
// api_key 必须为 ${ENV} 引用（plaintext_api_key 拒绝）
func (s ModelSpec) ResolveAPIKey() string // 展开 ${ENV}；非引用原样返回
```

### 组装

```go
type ModelFactory func(spec ModelSpec) (agentmodel.Model, error)

type Composed struct {
    Agent   *agentcore.Agent
    Tools   []agentcore.Tool
    Plugins *agentplugin.Registry
}

func Compose(ctx context.Context, m *AgentManifest, plugins *agentplugin.Registry, modelFactory ModelFactory) (*Composed, error)
// 流程：Validate → 从插件/ModelFactory 解析 model → 解析 tools → 构造 agent-core Agent
```

### `Builder`（Go fluent）

```go
type Builder struct{ /* ... */ }
func NewBuilder() *Builder
func (b *Builder) Name(name string) *Builder
func (b *Builder) Model(m agentmodel.Model) *Builder
func (b *Builder) Tools(ts ...agentcore.Tool) *Builder
func (b *Builder) OnEvent(f func(agentcore.Event)) *Builder
func (b *Builder) Build() (*agentcore.Agent, error) // model 缺失 / 工具名冲突报错
```

## 使用示例

### Manifest（YAML）

```yaml
apiVersion: v1
kind: agent
name: data-assistant
model:
  provider: openai-compatible
  base_url: ${MTS_BASEURL}
  api_key: ${MTS_API_KEY}   # 禁止明文
  model: gpt-5.4
tools:
  - file_reader
  - calculator
```

```go
m, _ := agentcompose.LoadManifest("agent.yaml")
composed, err := agentcompose.Compose(ctx, m, pluginRegistry, func(spec agentcompose.ModelSpec) (agentmodel.Model, error) {
    return modelopenai.New(modelopenai.Config{
        BaseURL: spec.BaseURL, APIKey: spec.ResolveAPIKey(), Model: spec.Model,
    })
})
result, err := composed.Agent.Run(ctx, "读取 data.json 计算 sales 总和")
```

### Builder（Go）

```go
agent, err := agentcompose.NewBuilder().
    Name("data-assistant").
    Model(client).
    Tools(tools.FileReader{}, tools.Calculator{}).
    Build()
```

## 边界

- 不包含具体模型 SDK；模型由调用方经 `ModelFactory` 注入。
- `Pattern`/`Permissions` 字段在 v0.1 由上层（`agent-runtime`/应用）消费。

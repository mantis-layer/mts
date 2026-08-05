// Package agentcompose 提供开发者友好的 Agent 组合层（G5）：
// Agent Manifest（YAML/JSON）加载、校验、组装，以及 Go Builder API。
package agentcompose

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
	agentplugin "github.com/mantis-layer/mts/agent-plugin"
	"gopkg.in/yaml.v3"
)

// CurrentAPIVersion 是 Agent Manifest 当前支持的 apiVersion。
const CurrentAPIVersion = "v1"

// ModelSpec 描述模型 Provider 配置。
type ModelSpec struct {
	// Provider 模型 Provider 插件名（plugins registry 中注册的 model_provider 插件）。
	Provider string `yaml:"provider" json:"provider"`
	// BaseURL OpenAI 兼容端点；仅在无 provider 插件时内联使用。
	BaseURL string `yaml:"base_url,omitempty" json:"base_url,omitempty"`
	// APIKey 支持 ${ENV_NAME} 环境变量引用，绝不写明文。
	APIKey string `yaml:"api_key,omitempty" json:"api_key,omitempty"`
	// Model 模型名称。
	Model string `yaml:"model" json:"model"`
}

// AgentManifest 声明一个 Agent 的组合（model + tools + pattern）。
type AgentManifest struct {
	APIVersion  string    `yaml:"api_version" json:"api_version"`
	Kind        string    `yaml:"kind" json:"kind"` // "agent"
	Name        string    `yaml:"name" json:"name"`
	Model       ModelSpec `yaml:"model" json:"model"`
	Tools       []string  `yaml:"tools" json:"tools"`
	Pattern     string    `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Permissions []string  `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// ManifestError 是结构化的 Manifest 校验错误。
type ManifestError struct {
	Code    string
	Message string
}

func (e *ManifestError) Error() string {
	return "agentcompose: " + e.Code + ": " + e.Message
}

// LoadManifest 从文件加载 Agent Manifest（YAML 或 JSON，按扩展名）。
func LoadManifest(path string) (*AgentManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("agentcompose: 读取 manifest: %w", err)
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("agentcompose: %s: %w", path, err)
	}
	return m, nil
}

// ParseManifest 解析 YAML 或 JSON 内容为 AgentManifest。
func ParseManifest(data []byte) (*AgentManifest, error) {
	var m AgentManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("解析 manifest: %w", err)
	}
	return &m, nil
}

// Validate 校验 Manifest 结构：apiVersion、kind、name、model 字段。
func (m *AgentManifest) Validate() error {
	if m.APIVersion != CurrentAPIVersion {
		return &ManifestError{Code: "unsupported_api_version", Message: fmt.Sprintf("apiVersion=%q（当前 %q）", m.APIVersion, CurrentAPIVersion)}
	}
	if m.Kind != "" && m.Kind != "agent" {
		return &ManifestError{Code: "invalid_kind", Message: fmt.Sprintf("kind=%q（期望 agent）", m.Kind)}
	}
	if m.Name == "" {
		return &ManifestError{Code: "missing_name", Message: "agent name 不能为空"}
	}
	if m.Model.Provider == "" {
		return &ManifestError{Code: "missing_provider", Message: "model.provider 不能为空"}
	}
	if m.Model.Model == "" {
		return &ManifestError{Code: "missing_model_name", Message: "model.model 不能为空"}
	}
	// 防明文密钥：api_key 若填写必须是 ${ENV} 引用形式（NFR-004）。
	if k := strings.TrimSpace(m.Model.APIKey); k != "" && !isEnvRef(k) {
		return &ManifestError{Code: "plaintext_api_key", Message: "model.api_key 必须使用 ${ENV_NAME} 引用，禁止明文密钥"}
	}
	return nil
}

// isEnvRef 判断字符串是否为 ${ENV_NAME} 引用形式。
func isEnvRef(s string) bool {
	if !strings.HasPrefix(s, "${") || !strings.HasSuffix(s, "}") {
		return false
	}
	name := strings.TrimSuffix(strings.TrimPrefix(s, "${"), "}")
	if name == "" {
		return false
	}
	for _, r := range name {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// ResolveAPIKey 展开 APIKey 中的 ${ENV} 引用；非引用形式（空/占位）原样返回。
func (s ModelSpec) ResolveAPIKey() string {
	v := strings.TrimSpace(s.APIKey)
	if strings.HasPrefix(v, "${") && strings.HasSuffix(v, "}") {
		name := strings.TrimSuffix(strings.TrimPrefix(v, "${"), "}")
		return os.Getenv(name)
	}
	return v
}

// ModelFactory 根据 ModelSpec 构造模型实现（用于 provider=openai-compatible，
// 由调用方注入对应 adapter，避免 agent-compose 反向依赖具体 adapter）。
type ModelFactory func(spec ModelSpec) (agentmodel.Model, error)

// Composed 是一次组装的结果。
type Composed struct {
	Agent   *agentcore.Agent
	Tools   []agentcore.Tool
	Plugins *agentplugin.Registry
}

// Compose 按 Manifest 组装 Agent（B5）：
// 校验 manifest → 从插件 registry 或 model factory 解析 model → 解析 tools → 构造 agent-core Agent。
func Compose(ctx context.Context, m *AgentManifest, plugins *agentplugin.Registry, modelFactory ModelFactory) (*Composed, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}

	// 解析模型：优先 provider 插件；openai-compatible 走调用方注入的 factory。
	var model agentmodel.Model
	if m.Model.Provider != "openai-compatible" {
		var ok bool
		model, ok = plugins.GetModel(m.Model.Provider)
		if !ok {
			return nil, &ManifestError{Code: "unknown_provider", Message: fmt.Sprintf("模型 provider %q 未注册", m.Model.Provider)}
		}
	} else {
		if modelFactory == nil {
			return nil, &ManifestError{Code: "missing_openai_model", Message: "provider=openai-compatible 需要提供 ModelFactory"}
		}
		var err error
		model, err = modelFactory(m.Model)
		if err != nil {
			return nil, &ManifestError{Code: "model_factory_error", Message: fmt.Sprintf("构造模型失败: %v", err)}
		}
		if model == nil {
			return nil, &ManifestError{Code: "model_factory_error", Message: "ModelFactory 返回 nil 模型"}
		}
	}

	// 解析工具：所有引用的工具必须已在插件中注册（B5）。
	toolReg := agentcore.NewRegistry()
	toolPlugins, err := collectToolPlugins(plugins)
	if err != nil {
		return nil, err
	}
	for _, name := range m.Tools {
		found := false
		for _, p := range toolPlugins {
			if t, ok := p.toolByName(name); ok {
				if err := toolReg.Register(t); err != nil {
					return nil, fmt.Errorf("agentcompose: 注册工具 %q: %w", name, err)
				}
				found = true
				break
			}
		}
		if !found {
			return nil, &ManifestError{Code: "unknown_tool", Message: fmt.Sprintf("manifest 引用的工具 %q 未在任何已注册插件中", name)}
		}
	}

	agent := agentcore.New(model, toolReg, agentcore.Options{})
	return &Composed{Agent: agent, Tools: toolReg.ListTools(), Plugins: plugins}, nil
}

// toolLookup 缓存插件内工具名到工具的映射。
type toolLookup struct {
	byID map[string]agentcore.Tool
}

func (l *toolLookup) toolByName(name string) (agentcore.Tool, bool) {
	t, ok := l.byID[name]
	return t, ok
}

func collectToolPlugins(plugins *agentplugin.Registry) ([]*toolLookup, error) {
	var out []*toolLookup
	for _, p := range plugins.Plugins() {
		tp, ok := p.(agentplugin.ToolPlugin)
		if !ok {
			continue
		}
		l := &toolLookup{byID: make(map[string]agentcore.Tool)}
		for _, t := range tp.Tools() {
			if _, dup := l.byID[t.Name()]; dup {
				return nil, fmt.Errorf("agentcompose: 插件 %q 提供重复工具 %q", p.Manifest().Name, t.Name())
			}
			l.byID[t.Name()] = t
		}
		out = append(out, l)
	}
	return out, nil
}

// MarshalJSON 供调试输出（APIKey 脱敏）。
func (m AgentManifest) MarshalJSON() ([]byte, error) {
	type safeModel struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url,omitempty"`
		APIKey   string `json:"api_key,omitempty"`
		Model    string `json:"model"`
	}
	safe := struct {
		APIVersion  string    `json:"api_version"`
		Kind        string    `json:"kind"`
		Name        string    `json:"name"`
		Model       safeModel `json:"model"`
		Tools       []string  `json:"tools"`
		Pattern     string    `json:"pattern,omitempty"`
		Permissions []string  `json:"permissions,omitempty"`
	}{
		APIVersion:  m.APIVersion,
		Kind:        m.Kind,
		Name:        m.Name,
		Tools:       m.Tools,
		Pattern:     m.Pattern,
		Permissions: m.Permissions,
		Model: safeModel{
			Provider: m.Model.Provider,
			BaseURL:  m.Model.BaseURL,
			Model:    m.Model.Model,
		},
	}
	if m.Model.APIKey != "" {
		safe.Model.APIKey = "[REDACTED]"
	}
	return json.Marshal(safe)
}

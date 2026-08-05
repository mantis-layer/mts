// Package agentplugin 提供正式插件扩展机制（FR-006）：类型化扩展点、
// Plugin Manifest、Registry、apiVersion/类型/版本校验与生命周期。
//
// 插件机制以静态 Go 注册为主（NFR-002），不依赖 Go 原生动态 plugin 包。
package agentplugin

// CurrentAPIVersion 是当前支持的插件公共协议版本（NFR-006）。
const CurrentAPIVersion = "v1"

// PluginType 标识插件扩展的类型。
type PluginType string

const (
	// PluginTypeModelProvider 模型 Provider 插件。
	PluginTypeModelProvider PluginType = "model_provider"
	// PluginTypeTool 工具集合插件。
	PluginTypeTool PluginType = "tool"
	// PluginTypeEvaluator 评估器插件。
	PluginTypeEvaluator PluginType = "evaluator"
	// PluginTypePattern 执行模式插件。
	PluginTypePattern PluginType = "pattern"
)

// PluginManifest 描述一个插件的元数据。
type PluginManifest struct {
	Name         string     `json:"name" yaml:"name"`
	Version      string     `json:"version" yaml:"version"`
	APIVersion   string     `json:"api_version" yaml:"api_version"`
	Type         PluginType `json:"type" yaml:"type"`
	Description  string     `json:"description,omitempty" yaml:"description,omitempty"`
	Permissions  []string   `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Capabilities []string   `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
}

// ManifestError 是结构化的 Manifest 校验错误。
type ManifestError struct {
	Code     string
	Message  string
	Manifest PluginManifest
}

func (e *ManifestError) Error() string {
	return "agentplugin: " + e.Code + ": " + e.Message
}

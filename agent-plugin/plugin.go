package agentplugin

import (
	"context"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Plugin 是所有插件的基础契约：声明 Manifest 并实现生命周期。
type Plugin interface {
	// Manifest 返回插件元数据。
	Manifest() PluginManifest
	// Init 在注册时调用；失败则插件不被注册。
	Init(ctx context.Context) error
	// Close 在 Agent/应用退出时调用，用于释放资源。
	Close() error
}

// ToolPlugin 提供一组工具集合的插件（扩展点：tools）。
type ToolPlugin interface {
	Plugin
	// Tools 返回插件提供的工具。
	Tools() []agentcore.Tool
}

// ModelProviderPlugin 提供模型实现的插件（扩展点：model provider）。
type ModelProviderPlugin interface {
	Plugin
	// Model 返回模型实现；失败则插件注册失败。
	Model() (agentmodel.Model, error)
}

// EvaluatorPlugin 提供评估器的插件（扩展点：evaluator）。
// v0.1 仅定义契约标记，Evaluator 类型由 Task Runtime 切片消费。
type EvaluatorPlugin interface {
	Plugin
	Evaluators() []any
}

// PatternPlugin 提供执行模式插件的扩展点（扩展点：pattern）。
type PatternPlugin interface {
	Plugin
	Patterns() []any
}

package agentcompose

import (
	"fmt"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Builder 是 Go API 的 fluent 组合器（G5），等效于 Manifest 组装。
type Builder struct {
	name    string
	model   agentmodel.Model
	tools   []agentcore.Tool
	onEvent func(agentcore.Event)
}

// NewBuilder 构造空 Builder。
func NewBuilder() *Builder { return &Builder{} }

// Name 设置 Agent 名称。
func (b *Builder) Name(name string) *Builder { b.name = name; return b }

// Model 设置模型实现。
func (b *Builder) Model(m agentmodel.Model) *Builder { b.model = m; return b }

// Tools 追加工具。
func (b *Builder) Tools(ts ...agentcore.Tool) *Builder { b.tools = append(b.tools, ts...); return b }

// OnEvent 设置事件回调。
func (b *Builder) OnEvent(f func(agentcore.Event)) *Builder { b.onEvent = f; return b }

// Build 构造 agent-core Agent；model 缺失或工具名冲突时报错。
func (b *Builder) Build() (*agentcore.Agent, error) {
	if b.model == nil {
		return nil, &ManifestError{Code: "missing_model", Message: "Build 需要先设置 Model"}
	}
	reg := agentcore.NewRegistry()
	seen := map[string]bool{}
	for _, t := range b.tools {
		if t == nil {
			continue
		}
		if seen[t.Name()] {
			return nil, &ManifestError{Code: "duplicate_tool", Message: fmt.Sprintf("工具 %q 重复添加", t.Name())}
		}
		seen[t.Name()] = true
		if err := reg.Register(t); err != nil {
			return nil, fmt.Errorf("agentcompose: %w", err)
		}
	}
	return agentcore.New(b.model, reg, agentcore.Options{OnEvent: b.onEvent}), nil
}

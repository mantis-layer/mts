package agentplugin

import (
	"context"
	"fmt"
	"sort"
	"sync"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Registry 是插件注册表：校验 Manifest → Init → 按类型提取扩展。
// 静态注册为主，插件注册后即可被组合层查询（FR-006）。
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	tools   map[string]agentcore.Tool
	models  map[string]agentmodel.Model
}

// NewRegistry 构造空插件注册表。
func NewRegistry() *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		tools:   make(map[string]agentcore.Tool),
		models:  make(map[string]agentmodel.Model),
	}
}

// Register 校验 Manifest 并注册插件；任一环节失败返回错误且不残留状态。
//
// 顺序：锁外完成类型断言 / Init / 扩展提取（可重入），
// 锁内仅做查重与写入（快速路径）；提交阶段失败时回滚已写入内容。
func (r *Registry) Register(ctx context.Context, p Plugin) error {
	if p == nil {
		return fmt.Errorf("agentplugin: 不能注册 nil 插件")
	}
	m := p.Manifest()
	if err := ValidateManifest(m); err != nil {
		return err
	}

	// —— 锁外预检与扩展提取 ——
	var tools []agentcore.Tool
	var model agentmodel.Model
	switch m.Type {
	case PluginTypeTool:
		tp, ok := p.(ToolPlugin)
		if !ok {
			return fmt.Errorf("agentplugin: 插件 %q 声明 type=tool 但未实现 ToolPlugin", m.Name)
		}
		tools = tp.Tools()
	case PluginTypeModelProvider:
		mp, ok := p.(ModelProviderPlugin)
		if !ok {
			return fmt.Errorf("agentplugin: 插件 %q 声明 type=model_provider 但未实现 ModelProviderPlugin", m.Name)
		}
		var err error
		model, err = mp.Model()
		if err != nil {
			return fmt.Errorf("agentplugin: 插件 %q Model() 失败: %w", m.Name, err)
		}
	}
	if err := p.Init(ctx); err != nil {
		return fmt.Errorf("agentplugin: 插件 %q Init 失败: %w", m.Name, err)
	}
	// Init 成功后若提交阶段失败，回滚时 Close 释放插件资源。
	needsRollback := true
	defer func() {
		if needsRollback {
			_ = p.Close()
		}
	}()

	// —— 锁内查重与提交（失败回滚） ——
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.plugins[m.Name]; dup {
		return &ManifestError{Code: "duplicate_name", Message: fmt.Sprintf("插件 %q 已注册", m.Name), Manifest: m}
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		if _, dup := r.tools[t.Name()]; dup {
			return &ManifestError{Code: "duplicate_tool", Message: fmt.Sprintf("插件 %q 注册了重复工具 %q", m.Name, t.Name()), Manifest: m}
		}
	}
	if model != nil {
		if _, dup := r.models[m.Name]; dup {
			return &ManifestError{Code: "duplicate_provider", Message: fmt.Sprintf("模型 Provider %q 已注册", m.Name), Manifest: m}
		}
	}

	// 提交：全部查重通过后才写入
	r.plugins[m.Name] = p
	for _, t := range tools {
		if t != nil {
			r.tools[t.Name()] = t
		}
	}
	if model != nil {
		r.models[m.Name] = model
	}
	needsRollback = false // 提交成功，不再 Close
	return nil
}

// Plugins 返回全部已注册插件（按名称排序）。
func (r *Registry) Plugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.plugins))
	for n := range r.plugins {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Plugin, 0, len(names))
	for _, n := range names {
		out = append(out, r.plugins[n])
	}
	return out
}

// GetPlugin 按名称取插件。
func (r *Registry) GetPlugin(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

// Tools 返回全部已注册工具（按名称排序）。
func (r *Registry) Tools() []agentcore.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]agentcore.Tool, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n])
	}
	return out
}

// GetTool 按名称取工具。
func (r *Registry) GetTool(name string) (agentcore.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// ToolSchemas 返回全部工具 Schema（供模型调用）。
func (r *Registry) ToolSchemas() []agentmodel.ToolSchema {
	tools := r.Tools()
	out := make([]agentmodel.ToolSchema, 0, len(tools))
	for _, t := range tools {
		out = append(out, agentmodel.NewToolSchema(t.Name(), t.Description(), t.Parameters()))
	}
	return out
}

// Models 返回全部已注册模型 Provider（按名称）。
func (r *Registry) Models() map[string]agentmodel.Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]agentmodel.Model, len(r.models))
	for n, m := range r.models {
		out[n] = m
	}
	return out
}

// GetModel 按名称取模型 Provider。
func (r *Registry) GetModel(name string) (agentmodel.Model, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.models[name]
	return m, ok
}

// CloseAll 关闭全部插件（逆序），返回聚合错误。
func (r *Registry) CloseAll() error {
	plugins := r.Plugins()
	var errs []error
	for i := len(plugins) - 1; i >= 0; i-- {
		if err := plugins[i].Close(); err != nil {
			errs = append(errs, fmt.Errorf("插件 %q Close 失败: %w", plugins[i].Manifest().Name, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("agentplugin: %d 个插件关闭失败: %v", len(errs), errs)
	}
	return nil
}

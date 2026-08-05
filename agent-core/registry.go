package agentcore

import (
	"fmt"
	"sort"
	"sync"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Registry 是静态工具注册表。v0.1 只支持静态注册（后续切片引入 Plugin System）。
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register 注册工具；Name 重复时返回错误（唯一 ID 约束，FR-003）。
func (r *Registry) Register(t Tool) error {
	if t == nil {
		return fmt.Errorf("registry: 不能注册 nil 工具")
	}
	name := t.Name()
	if name == "" {
		return fmt.Errorf("registry: 工具 Name 不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("registry: 工具 %q 已注册", name)
	}
	r.tools[name] = t
	return nil
}

// Get 按名称取工具。
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// Len 返回已注册工具数量。
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tools)
}

// ListTools 返回全部已注册工具（按名称排序）。
func (r *Registry) ListTools() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Tool, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n])
	}
	return out
}

// Schemas 返回全部工具的描述 Schema（供模型调用，FR-001/FR-003）。
func (r *Registry) Schemas() []agentmodel.ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]agentmodel.ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, agentmodel.NewToolSchema(t.Name(), t.Description(), t.Parameters()))
	}
	return out
}

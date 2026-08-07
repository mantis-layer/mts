package agentcontract

import (
	"context"
	"fmt"
	"time"
)

// MemoryLayer 标识记忆所在的分层（FR-011，五层冻结）。
type MemoryLayer string

const (
	MemoryLayerWorking    MemoryLayer = "working"
	MemoryLayerShortTerm  MemoryLayer = "shortterm"
	MemoryLayerLongTerm   MemoryLayer = "longterm"
	MemoryLayerPreference MemoryLayer = "preference"
	MemoryLayerSkill      MemoryLayer = "skill"
)

var validMemoryLayers = map[MemoryLayer]struct{}{
	MemoryLayerWorking:    {},
	MemoryLayerShortTerm:  {},
	MemoryLayerLongTerm:   {},
	MemoryLayerPreference: {},
	MemoryLayerSkill:      {},
}

// Valid 报告 layer 是否为五层 enum 中的合法值。
func (l MemoryLayer) Valid() bool {
	_, ok := validMemoryLayers[l]
	return ok
}

// Validate 返回非法 MemoryLayer 的明确错误。
func (l MemoryLayer) Validate() error {
	if !l.Valid() {
		return fmt.Errorf("agentcontract: 非法 MemoryLayer %q（允许: working/shortterm/longterm/preference/skill）", l)
	}
	return nil
}

// Memory 是 Persona 的一条记忆（FR-011）。
// Embedding 允许为 nil：Working 层不向量化，Query 时区分。
type Memory struct {
	ID        string
	PersonaID string
	Layer     MemoryLayer
	Content   string
	Metadata  map[string]any // 层相关的补充元数据（精确字段由 D5 示例驱动冻结）
	Tags      []string
	Embedding []float32 // nil 表示未向量化（Working 层）
	CreatedAt time.Time
}

// Validate 校验 Memory 的最小契约：PersonaID 非空、Layer 合法；
// Embedding 允许为 nil（Working 层不做向量化）。
func (m Memory) Validate() error {
	if m.PersonaID == "" {
		return fmt.Errorf("agentcontract: Memory.PersonaID 不能为空")
	}
	return m.Layer.Validate()
}

// QueryOptions 是 MemoryStore.Query 的过滤/分页选项（保持最小，D5 按示例回填）。
type QueryOptions struct {
	Limit int      // 返回条数上限；0 表示不限制
	Tags  []string // 标签过滤
	// QueryText 是向量检索的查询文本（FR-011，D5 回填）。
	// 非 Working 层、且 store 配置了 EmbeddingProvider 时，Query 会对该文本
	// 生成查询向量做余弦相似度 Top-K；为空时退化为 created_at 倒序。
	QueryText string
}

// MemoryStore 是 Persona 记忆的存取接口（FR-011），可被插件替换实现。
type MemoryStore interface {
	// Save 保存一条记忆（包含更新）。
	Save(ctx context.Context, m *Memory) error
	// Query 按 persona + layer 查询记忆，按相关度/时间由实现决定排序。
	Query(ctx context.Context, personaID string, layer MemoryLayer, opts QueryOptions) ([]Memory, error)
	// Delete 按记忆 ID 删除。
	Delete(ctx context.Context, id string) error
}

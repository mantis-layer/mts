package integration

import (
	"context"
	"fmt"
	"time"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"
	agentruntime "github.com/mantis-layer/mts/agent-runtime"
)

// PersonaSpec 是构造一个示例 Persona 的最小参数（三示例共用）。
type PersonaSpec struct {
	ID           string
	Name         string
	Role         string
	SystemPrompt string
}

// Env 是一次会话的共享身份 + 记忆 + 注意力三件套（S11/P1）：
// Persona（身份）+ VectorMemoryStore（记忆）+ DefaultContextBuilder（注意力）。
//
// 三示例都用同一个 setupSession 构造这三者，避免重复造轮子——这是 Issue #46
// 验收 P1/S11 的核心：共享抽象、用法一致，无需修改 agent-core。
type Env struct {
	Persona         *agentcontract.Persona
	MemoryStore     *agentruntime.VectorMemoryStore
	ContextBuilder  *agentruntime.DefaultContextBuilder
	MemoryStorePath string
}

// setupSession 按 PRD v3 的统一用法构造 Persona + MemoryStore + ContextBuilder。
//
//   - memoryPath：SQLite 文件路径（跨会话持久化）；传 ":memory:" 为进程内临时库。
//   - embed：EmbeddingProvider，可传 nil（此时 VectorMemoryStore 退化为规则检索）。
//   - spec：Persona 的身份字段。
//
// 返回的 Env 持有可关闭的 MemoryStore；调用方负责 Close（跨会话恢复测试除外）。
func setupSession(memoryPath string, embed agentmodel.EmbeddingProvider, spec PersonaSpec) (*Env, error) {
	now := time.Now()
	persona := &agentcontract.Persona{
		ID:           spec.ID,
		Name:         spec.Name,
		Role:         spec.Role,
		SystemPrompt: spec.SystemPrompt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := persona.Validate(); err != nil {
		return nil, err
	}
	mem, err := agentruntime.NewVectorMemoryStore(memoryPath, embed, EmbedDim)
	if err != nil {
		return nil, fmt.Errorf("integration: 创建 MemoryStore: %w", err)
	}
	cb := agentruntime.NewDefaultContextBuilder()
	return &Env{
		Persona:         persona,
		MemoryStore:     mem,
		ContextBuilder:  cb,
		MemoryStorePath: memoryPath,
	}, nil
}

// saveMemory 是写一条记忆到 MemoryStore 的便捷方法（三示例统一用法）。
func saveMemory(ctx context.Context, store *agentruntime.VectorMemoryStore, personaID string, layer agentcontract.MemoryLayer, content string, tags ...string) error {
	m := &agentcontract.Memory{
		ID:        fmt.Sprintf("%s-%s-%d", personaID, layer, time.Now().UnixNano()),
		PersonaID: personaID,
		Layer:     layer,
		Content:   content,
		Tags:      tags,
		CreatedAt: time.Now(),
	}
	if err := m.Validate(); err != nil {
		return err
	}
	return store.Save(ctx, m)
}

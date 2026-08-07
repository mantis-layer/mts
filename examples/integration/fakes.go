// Package integration 提供 Issue #46 的端到端集成验收测试（S11/S13/S14）。
//
// 它证明三个已合并子节点（#43 Persona / #44 vector-memory / #45 context-builder）
// 在无需修改 agent-core 源码的前提下，可被组合使用：
//   - 三示例共享的 Persona + MemoryStore + ContextBuilder 抽象（A1/A2/S11/P1）
//   - 跨会话恢复：Run1 写 LongTerm 记忆 → 进程退出 → Run2 按同一 PersonaID 检索到（A3/S13/P2）
//   - 记忆注入事件可观测：MemoryInjected 事件在每次模型调用前触发（A4/S14/P3）
//
// 全部测试离线、确定：fakeModel 返回固定文本、fakeEmbedder 用确定性词袋哈希向量，
// 不依赖真实 OpenAI key 与网络。跨会话持久化用临时 SQLite 文件。
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// EmbedDim 是测试用 embedding 维度。选 16（足够区分词袋相似度，开销低）。
const EmbedDim = 16

// fakeEmbedder 是 agentmodel.EmbeddingProvider 的确定性实现：词袋哈希向量。
//
// 策略（保证余弦相似度反映词重叠）：
//   - 按 ASCII 空白切词，每词小写；
//   - 每个词经 FNV-1a 哈希映射到 [0,EmbedDim) 的某个维度并 +1（词频）；
//   - 再用 SHA-256 种子做一次位置无关扰动，保证相同输入恒同向量（确定性）；
//   - 末尾 L2 归一化，使余弦相似度等价于向量点积。
//
// 结果：相同文本恒同向量；共享若干词的两段文本余弦相似度高（>0 时可被 Top-K 命中）。
type fakeEmbedder struct{}

// NewFakeEmbedder 返回确定性 EmbeddingProvider。
func NewFakeEmbedder() agentmodel.EmbeddingProvider { return fakeEmbedder{} }

// Embed 批量生成向量（确定性）。
func (fakeEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, 0, len(inputs))
	for _, s := range inputs {
		out = append(out, embedText(s))
	}
	return out, nil
}

// embedText 把单段文本编码为 EmbedDim 维 L2 归一化向量。
func embedText(s string) []float32 {
	vec := make([]float32, EmbedDim)
	tokens := strings.Fields(strings.ToLower(s))
	for _, t := range tokens {
		h := fnv.New64a()
		_, _ = h.Write([]byte(t))
		idx := int(h.Sum64() % uint64(EmbedDim))
		vec[idx] += 1.0
	}
	// 确定性扰动：避免完全相同的词袋集被映射到同一向量但不同语义。
	// 用 SHA-256 把整串混入末位附近的维度，仍保留词重叠主导的相似度。
	sum := sha256.Sum256([]byte(strings.ToLower(s)))
	seed := binary.BigEndian.Uint64(sum[:8])
	if EmbedDim > 0 {
		vec[int(seed%uint64(EmbedDim))] += 0.001
	}
	// L2 归一化。
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		inv := 1.0 / norm
		sqrtInv := inv
		// Newton 迭代一次足够（向量维小，误差不影响余弦排序）。
		sqrtInv = 0.5 * (sqrtInv + inv*sqrtInv)
		for i := range vec {
			vec[i] = float32(float64(vec[i]) * sqrtInv)
		}
	}
	return vec
}

// fakeModel 是 agentmodel.Model 的离线确定性实现。
// 它按预置的流序列返回，并记录每次调用收到的消息视图（供测试断言记忆注入是否发生）。
type fakeModel struct {
	mu       sync.Mutex
	streams  [][]agentmodel.StreamEvent // 逐次调用的预置响应流
	calls    int                        // 已发生的模型调用次数
	views    [][]agentmodel.Message     // 每次调用收到的消息视图（含注入的记忆）
	streamNo func(callIdx int) int      // 可选：自定义选流索引（默认按 calls 顺序）
}

// newFakeModel 构造一个 fakeModel，按 streams 顺序返回响应。
func newFakeModel(streams ...[]agentmodel.StreamEvent) *fakeModel {
	return &fakeModel{streams: streams}
}

func (m *fakeModel) ModelName() string { return "fake" }

func (m *fakeModel) Complete(ctx context.Context, req agentmodel.Request) (agentmodel.Response, error) {
	evs := m.nextAndRecord(ctx, req)
	assistant := agentmodel.Message{Role: agentmodel.RoleAssistant}
	var usage agentmodel.Usage
	for _, ev := range evs {
		switch ev.Kind {
		case agentmodel.StreamEventDelta:
			assistant.Content += ev.Delta
		case agentmodel.StreamEventToolCall:
			if ev.ToolCall != nil {
				assistant.ToolCalls = append(assistant.ToolCalls, *ev.ToolCall)
			}
		case agentmodel.StreamEventUsage:
			if ev.Usage != nil {
				usage = *ev.Usage
			}
		}
	}
	return agentmodel.Response{Message: assistant, Usage: usage, FinishReason: agentmodel.FinishReasonStop}, nil
}

func (m *fakeModel) Stream(ctx context.Context, req agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	evs := m.nextAndRecord(ctx, req)
	ch := make(chan agentmodel.StreamEvent, len(evs)+1)
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch, nil
}

// nextAndRecord 取下一次响应流，并记录请求消息视图。
func (m *fakeModel) nextAndRecord(_ context.Context, req agentmodel.Request) []agentmodel.StreamEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	// 记录视图快照（深拷贝以避免外部修改）。
	view := make([]agentmodel.Message, len(req.Messages))
	copy(view, req.Messages)
	m.views = append(m.views, view)
	idx := m.calls - 1
	if m.streamNo != nil {
		idx = m.streamNo(m.calls) - 1
	}
	if idx < 0 || idx >= len(m.streams) {
		return []agentmodel.StreamEvent{{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop}}
	}
	return m.streams[idx]
}

// viewsSnapshot 返回当前累积的消息视图副本。
func (m *fakeModel) viewsSnapshot() [][]agentmodel.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]agentmodel.Message, len(m.views))
	for i, v := range m.views {
		cp := make([]agentmodel.Message, len(v))
		copy(cp, v)
		out[i] = cp
	}
	return out
}

// callCount 返回模型被调用的次数。
func (m *fakeModel) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// textStream 构造一段纯文本响应流（无工具调用，直接结束）。
func textStream(text string) []agentmodel.StreamEvent {
	return []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventDelta, Delta: text},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop},
	}
}

// toolCallStream 构造一段工具调用响应流。
func toolCallStream(name, args string) []agentmodel.StreamEvent {
	return []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: fmt.Sprintf("call_%s", name), Name: name, Arguments: args}},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
	}
}

package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// FR-012 默认 ContextBuilder 的 Spike 决策：
//   - 消息形态：注入为一条独立 system 消息（置于对话之前），而非 system 拼接；
//   - token 预算：固定预算（默认 2000），估算规则 4 字符 ≈ 1 token（无 tokenizer 依赖）；
//   - 检索层：longterm / preference / skill（working 是当前上下文，不向量化，R3）；
//   - 检索排序语义由 MemoryStore 实现决定（D5 VectorMemoryStore 提供向量相关度 Top-K）；
//     当前消息 msg 暂不参与检索（作为 D5 语义检索的衔接点）。
const (
	defaultMemoryQueryTimeout  = 2 * time.Second
	defaultMemoryTokenBudget   = 2000
	defaultMemoryPerLayerLimit = 5
	defaultMemoryCharsPerToken = 4
	memoryHeader               = "以下是 Agent 的相关记忆，供回答时参考：\n"
)

var defaultMemoryLayers = []agentcontract.MemoryLayer{
	agentcontract.MemoryLayerLongTerm,
	agentcontract.MemoryLayerPreference,
	agentcontract.MemoryLayerSkill,
}

// DefaultContextBuilder 是 FR-012 的默认 ContextBuilder 实现：
// 基于 Persona + MemoryStore 按层检索相关记忆，按 token 估算预算裁剪后，
// 注入为一条 system 消息。
type DefaultContextBuilder struct {
	// QueryTimeout 单层检索超时（默认 2s）；超时/失败返回错误，由调用方降级为不注入。
	QueryTimeout time.Duration
	// TokenBudget 注入内容的 token 估算预算（默认 2000）；超预算按序裁剪。
	TokenBudget int
	// Layers 检索的记忆层（默认 longterm/preference/skill）。
	Layers []agentcontract.MemoryLayer
	// PerLayerLimit 每层检索条数上限（默认 5）。
	PerLayerLimit int
}

// NewDefaultContextBuilder 返回带默认参数的 DefaultContextBuilder。
func NewDefaultContextBuilder() *DefaultContextBuilder {
	return &DefaultContextBuilder{
		QueryTimeout:  defaultMemoryQueryTimeout,
		TokenBudget:   defaultMemoryTokenBudget,
		Layers:        append([]agentcontract.MemoryLayer(nil), defaultMemoryLayers...),
		PerLayerLimit: defaultMemoryPerLayerLimit,
	}
}

// Build 检索 Persona 的相关记忆并注入为 system 消息（FR-012）。
// 检索无结果时返回空消息（nil error），表示"无注入"；
// 任一层检索失败/超时返回错误，由调用方降级为不注入（不阻塞主循环）。
// 注入内容按 TokenBudget 裁剪：TokenBudget 仅计入记忆内容本身
// （不含固定头与层标签开销），按层配置顺序与 Query 返回顺序（相关度）逐条累加，
// 超出预算的单条内容截断到剩余预算内。
func (b *DefaultContextBuilder) Build(ctx context.Context, p agentcontract.Persona, store agentcontract.MemoryStore, _ agentmodel.Message) (agentmodel.Message, error) {
	if err := p.Validate(); err != nil {
		return agentmodel.Message{}, err
	}
	if store == nil {
		return agentmodel.Message{}, errors.New("agentruntime: MemoryStore 未配置")
	}
	layers := b.Layers
	if len(layers) == 0 {
		return agentmodel.Message{}, nil
	}
	budget := b.TokenBudget
	if budget <= 0 {
		budget = defaultMemoryTokenBudget
	}
	limit := b.PerLayerLimit
	if limit <= 0 {
		limit = defaultMemoryPerLayerLimit
	}
	timeout := b.QueryTimeout
	if timeout <= 0 {
		timeout = defaultMemoryQueryTimeout
	}

	var parts []string
	used := 0
outer:
	for _, layer := range layers {
		qctx, cancel := context.WithTimeout(ctx, timeout)
		mems, err := store.Query(qctx, p.ID, layer, agentcontract.QueryOptions{Limit: limit})
		cancel()
		if err != nil {
			return agentmodel.Message{}, fmt.Errorf("agentruntime: 检索 %s 记忆失败: %w", layer, err)
		}
		for _, m := range mems {
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			est := estimateTokens(m.Content)
			if used+est > budget {
				if remaining := budget - used; remaining > 0 {
					t := truncateToTokens(m.Content, remaining)
					parts = append(parts, formatMemory(layer, t))
					used += estimateTokens(t)
				}
				break outer
			}
			parts = append(parts, formatMemory(layer, m.Content))
			used += est
		}
	}
	if len(parts) == 0 {
		return agentmodel.Message{}, nil
	}
	content := memoryHeader + strings.Join(parts, "\n")
	return agentmodel.Message{Role: agentmodel.RoleSystem, Content: content}, nil
}

func formatMemory(layer agentcontract.MemoryLayer, content string) string {
	return fmt.Sprintf("- [%s] %s", layer, content)
}

// estimateTokens 以 4 字符 ≈ 1 token 的启发式估算（Spike 决策，无 tokenizer 依赖）。
func estimateTokens(s string) int {
	n := utf8.RuneCountInString(s)
	return (n + defaultMemoryCharsPerToken - 1) / defaultMemoryCharsPerToken
}

// truncateToTokens 按 token 预算截断字符串，保持完整字符（不切分 rune）；
// 省略号预留 1 字符，保证估算结果不超预算。
func truncateToTokens(s string, tokens int) string {
	maxChars := tokens * defaultMemoryCharsPerToken
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars-1]) + "…"
}

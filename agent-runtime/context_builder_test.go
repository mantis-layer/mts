package agentruntime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// memStore 内存版 MemoryStore 测试实现。
type memStore struct {
	byLayer map[agentcontract.MemoryLayer][]agentcontract.Memory
	queryFn func(ctx context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions) ([]agentcontract.Memory, error)
}

func (s *memStore) Save(_ context.Context, _ *agentcontract.Memory) error { return nil }

func (s *memStore) Query(ctx context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
	if s.queryFn != nil {
		return s.queryFn(ctx, personaID, layer, opts)
	}
	return s.byLayer[layer], nil
}

func (s *memStore) Delete(_ context.Context, _ string) error { return nil }

var (
	_ agentcontract.MemoryStore    = (*memStore)(nil)
	_ agentcontract.ContextBuilder = (*DefaultContextBuilder)(nil)
)

func newTestPersona() agentcontract.Persona {
	return agentcontract.Persona{ID: "persona-1", Name: "测试伙伴", Role: "assistant"}
}

func TestDefaultContextBuilder_Defaults(t *testing.T) {
	b := NewDefaultContextBuilder()
	if b.QueryTimeout <= 0 {
		t.Fatalf("QueryTimeout=%v 应为正数", b.QueryTimeout)
	}
	if b.TokenBudget <= 0 {
		t.Fatalf("TokenBudget=%d 应为正数", b.TokenBudget)
	}
	if b.PerLayerLimit <= 0 {
		t.Fatalf("PerLayerLimit=%d 应为正数", b.PerLayerLimit)
	}
	found := false
	for _, l := range b.Layers {
		if l == agentcontract.MemoryLayerLongTerm {
			found = true
		}
	}
	if !found {
		t.Fatalf("默认 Layers 应包含 longterm，实际 %v", b.Layers)
	}
}

func TestDefaultContextBuilder_InjectRelevantMemories(t *testing.T) {
	store := &memStore{byLayer: map[agentcontract.MemoryLayer][]agentcontract.Memory{
		agentcontract.MemoryLayerLongTerm:   {{ID: "m1", PersonaID: "persona-1", Layer: agentcontract.MemoryLayerLongTerm, Content: "用户偏好简洁回答"}},
		agentcontract.MemoryLayerPreference: {{ID: "m2", PersonaID: "persona-1", Layer: agentcontract.MemoryLayerPreference, Content: "喜欢中文回复"}},
		agentcontract.MemoryLayerSkill:      {},
	}}
	b := NewDefaultContextBuilder()
	got, err := b.Build(context.Background(), newTestPersona(), store, agentmodel.Message{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Role != agentmodel.RoleSystem {
		t.Fatalf("注入消息角色=%q 期望 system", got.Role)
	}
	if !strings.Contains(got.Content, "用户偏好简洁回答") || !strings.Contains(got.Content, "喜欢中文回复") {
		t.Fatalf("注入内容缺失记忆: %q", got.Content)
	}
	if !strings.Contains(got.Content, "[longterm]") || !strings.Contains(got.Content, "[preference]") {
		t.Fatalf("注入内容应带层标签: %q", got.Content)
	}
}

func TestDefaultContextBuilder_QueryArgs(t *testing.T) {
	var mu struct {
		personaID string
		limit     int
		layers    map[agentcontract.MemoryLayer]int
	}
	mu.layers = map[agentcontract.MemoryLayer]int{}
	store := &memStore{queryFn: func(_ context.Context, personaID string, layer agentcontract.MemoryLayer, opts agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
		mu.personaID = personaID
		mu.limit = opts.Limit
		mu.layers[layer]++
		return nil, nil
	}}
	b := NewDefaultContextBuilder()
	if _, err := b.Build(context.Background(), newTestPersona(), store, agentmodel.Message{}); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if mu.personaID != "persona-1" || mu.limit != defaultMemoryPerLayerLimit {
		t.Fatalf("Query 参数 personaID=%q limit=%d", mu.personaID, mu.limit)
	}
	for _, l := range b.Layers {
		if mu.layers[l] != 1 {
			t.Fatalf("层 %s 应被检索 1 次，实际 %d", l, mu.layers[l])
		}
	}
}

func TestDefaultContextBuilder_TokenBudgetTruncation(t *testing.T) {
	long := strings.Repeat("很", 80) // 80 字符 ≈ 20 token
	store := &memStore{byLayer: map[agentcontract.MemoryLayer][]agentcontract.Memory{
		agentcontract.MemoryLayerLongTerm: {
			{ID: "m1", PersonaID: "persona-1", Layer: agentcontract.MemoryLayerLongTerm, Content: long},
			{ID: "m2", PersonaID: "persona-1", Layer: agentcontract.MemoryLayerLongTerm, Content: "第二条记忆不该进入预算内"},
		},
	}}
	b := NewDefaultContextBuilder()
	b.TokenBudget = 10 // 40 字符预算：第一条记忆截断，第二条整个丢弃
	got, err := b.Build(context.Background(), newTestPersona(), store, agentmodel.Message{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(got.Content, "…") {
		t.Fatalf("超预算记忆应被截断: %q", got.Content)
	}
	if strings.Contains(got.Content, "第二条记忆") {
		t.Fatalf("超出预算的后续记忆不应注入: %q", got.Content)
	}
	memPart := strings.TrimPrefix(strings.TrimPrefix(got.Content, memoryHeader), "- [longterm] ")
	if est := estimateTokens(memPart); est > b.TokenBudget {
		t.Fatalf("注入记忆估算 %d token 超出预算 %d: %q", est, b.TokenBudget, memPart)
	}
}

func TestDefaultContextBuilder_EmptyStoreNoInjection(t *testing.T) {
	b := NewDefaultContextBuilder()
	got, err := b.Build(context.Background(), newTestPersona(), &memStore{}, agentmodel.Message{})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got.Content != "" {
		t.Fatalf("空存储应无注入，实际 %q", got.Content)
	}
}

func TestDefaultContextBuilder_QueryTimeoutReturnsError(t *testing.T) {
	store := &memStore{queryFn: func(ctx context.Context, _ string, _ agentcontract.MemoryLayer, _ agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	b := NewDefaultContextBuilder()
	b.QueryTimeout = 50 * time.Millisecond
	start := time.Now()
	_, err := b.Build(context.Background(), newTestPersona(), store, agentmodel.Message{})
	if err == nil {
		t.Fatal("超时应返回错误")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("超时降级耗时过长: %v", elapsed)
	}
}

func TestDefaultContextBuilder_StoreErrorPropagates(t *testing.T) {
	want := errors.New("存储故障")
	store := &memStore{queryFn: func(context.Context, string, agentcontract.MemoryLayer, agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
		return nil, want
	}}
	b := NewDefaultContextBuilder()
	_, err := b.Build(context.Background(), newTestPersona(), store, agentmodel.Message{})
	if !errors.Is(err, want) {
		t.Fatalf("Build 错误=%v 应包含 %v", err, want)
	}
}

func TestDefaultContextBuilder_UnconfiguredStore(t *testing.T) {
	b := NewDefaultContextBuilder()
	if _, err := b.Build(context.Background(), newTestPersona(), nil, agentmodel.Message{}); err == nil {
		t.Fatal("store 为 nil 应返回错误")
	}
}

func TestDefaultContextBuilder_InvalidPersona(t *testing.T) {
	b := NewDefaultContextBuilder()
	if _, err := b.Build(context.Background(), agentcontract.Persona{}, &memStore{}, agentmodel.Message{}); err == nil {
		t.Fatal("Persona.ID 为空应返回错误")
	}
}

func TestDefaultContextBuilder_ContextCancelPropagates(t *testing.T) {
	store := &memStore{queryFn: func(ctx context.Context, _ string, _ agentcontract.MemoryLayer, _ agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	b := NewDefaultContextBuilder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := b.Build(ctx, newTestPersona(), store, agentmodel.Message{}); err == nil {
		t.Fatal("外部取消应返回错误")
	}
}

package agentcontract

import (
	"context"
	"strings"
	"testing"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// fakeStore 实现 MemoryStore，用于编译期与行为测试。
type fakeStore struct {
	saved   []Memory
	queried bool
	deleted []string
}

func (s *fakeStore) Save(_ context.Context, m *Memory) error {
	if m != nil {
		s.saved = append(s.saved, *m)
	}
	return nil
}

func (s *fakeStore) Query(_ context.Context, _ string, _ MemoryLayer, _ QueryOptions) ([]Memory, error) {
	s.queried = true
	return nil, nil
}

func (s *fakeStore) Delete(_ context.Context, id string) error {
	s.deleted = append(s.deleted, id)
	return nil
}

// fakeBuilder 实现 ContextBuilder。
type fakeBuilder struct{}

func (fakeBuilder) Build(_ context.Context, p Persona, _ MemoryStore, msg agentmodel.Message) (agentmodel.Message, error) {
	msg.Content = "[" + p.ID + "]" + msg.Content
	return msg, nil
}

// 编译期接口满足性检查。
var (
	_ MemoryStore    = (*fakeStore)(nil)
	_ ContextBuilder = fakeBuilder{}
)

func TestMemoryLayerEnumValues(t *testing.T) {
	want := []MemoryLayer{
		MemoryLayerWorking,
		MemoryLayerShortTerm,
		MemoryLayerLongTerm,
		MemoryLayerPreference,
		MemoryLayerSkill,
	}
	if len(want) != 5 {
		t.Fatalf("MemoryLayer 应为五层，实际 %d 个常量", len(want))
	}
	for _, l := range want {
		if !l.Valid() {
			t.Errorf("MemoryLayer %q 应合法", l)
		}
	}
	unique := map[MemoryLayer]bool{}
	for _, l := range want {
		unique[l] = true
	}
	if len(unique) != 5 {
		t.Errorf("MemoryLayer 常量存在重复值: %v", want)
	}
}

func TestMemoryLayerInvalidValue(t *testing.T) {
	bad := MemoryLayer("episodic")
	if bad.Valid() {
		t.Fatalf("MemoryLayer %q 不应合法", bad)
	}
	err := bad.Validate()
	if err == nil {
		t.Fatal("非法 MemoryLayer 应返回错误")
	}
	if !strings.Contains(err.Error(), "episodic") {
		t.Errorf("错误应包含非法值本身，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "working") {
		t.Errorf("错误应列出允许值，实际: %v", err)
	}
}

func TestPersonaConstructionAndValidation(t *testing.T) {
	now := time.Now()
	p := Persona{
		ID:           "persona-1",
		Name:         "Alice",
		Role:         "analyst",
		SystemPrompt: "你是数据分析助手",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("合法 Persona 校验失败: %v", err)
	}
	if p.ID != "persona-1" || p.Name != "Alice" || p.Role != "analyst" {
		t.Errorf("Persona 字段构造不符: %+v", p)
	}
	if got := (Persona{}).Validate(); got == nil {
		t.Error("PersonaID 为空应校验失败")
	}
}

func TestMemoryConstructionAndValidation(t *testing.T) {
	m := Memory{
		ID:        "mem-1",
		PersonaID: "persona-1",
		Layer:     MemoryLayerWorking,
		Content:   "本轮上下文",
		Tags:      []string{"session"},
		CreatedAt: time.Now(),
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Working 层记忆校验失败: %v", err)
	}
	if m.Embedding != nil {
		t.Error("Working 层记忆的 Embedding 应为 nil（默认未向量化）")
	}
}

func TestMemoryEmbeddingNilAllowed(t *testing.T) {
	m := Memory{
		PersonaID: "persona-1",
		Layer:     MemoryLayerWorking,
		Content:   "未向量化的工作记忆",
		Embedding: nil,
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("Embedding 为 nil 应允许（Working 层）: %v", err)
	}
}

func TestMemoryValidationRejects(t *testing.T) {
	if got := (Memory{}).Validate(); got == nil {
		t.Error("Memory.PersonaID 为空应校验失败")
	}
	m := Memory{
		PersonaID: "persona-1",
		Layer:     MemoryLayer("episodic"),
	}
	if got := m.Validate(); got == nil {
		t.Error("Memory 的非法 Layer 应校验失败")
	} else if !strings.Contains(got.Error(), "episodic") {
		t.Errorf("错误应包含非法 Layer，实际: %v", got)
	}
}

func TestMemoryStoreInterface(t *testing.T) {
	ctx := context.Background()
	s := &fakeStore{}
	m := &Memory{ID: "mem-1", PersonaID: "persona-1", Layer: MemoryLayerWorking}
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save 失败: %v", err)
	}
	if len(s.saved) != 1 || s.saved[0].ID != "mem-1" {
		t.Errorf("Save 未落盘到 fake: %+v", s.saved)
	}
	if _, err := s.Query(ctx, "persona-1", MemoryLayerWorking, QueryOptions{Limit: 5}); err != nil {
		t.Fatalf("Query 失败: %v", err)
	}
	if !s.queried {
		t.Error("Query 未调用 fake 实现")
	}
	if err := s.Delete(ctx, "mem-1"); err != nil {
		t.Fatalf("Delete 失败: %v", err)
	}
	if len(s.deleted) != 1 || s.deleted[0] != "mem-1" {
		t.Errorf("Delete 未记录: %v", s.deleted)
	}
}

func TestContextBuilderInterface(t *testing.T) {
	ctx := context.Background()
	var b ContextBuilder = fakeBuilder{}
	out, err := b.Build(ctx, Persona{ID: "persona-1"}, &fakeStore{}, agentmodel.Message{
		Role:    agentmodel.RoleUser,
		Content: "你好",
	})
	if err != nil {
		t.Fatalf("Build 失败: %v", err)
	}
	if out.Content != "[persona-1]你好" {
		t.Errorf("Build 未注入 Persona 上下文: %q", out.Content)
	}
}

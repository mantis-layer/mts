package agentruntime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mantis-layer/mts/agent-contract"
)

// fakeEmbedProvider 是确定性 EmbeddingProvider：把文本词频映射到固定维度向量，
// 使语义相近的文本余弦相似度高。用于覆盖验收 A2/A4/A5，不依赖真实模型。
// 维度 = 词汇表大小；每个词贡献一个独立维，词频为该维值（稀疏 bag-of-words）。
type fakeEmbedProvider struct {
	dim   int
	vocab map[string]int // word -> 维度下标
}

func newFakeEmbedProvider(vocab []string) *fakeEmbedProvider {
	m := make(map[string]int, len(vocab))
	for i, w := range vocab {
		m[strings.ToLower(w)] = i
	}
	return &fakeEmbedProvider{dim: len(vocab), vocab: m}
}

func (f *fakeEmbedProvider) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	out := make([][]float32, len(inputs))
	for i, s := range inputs {
		v := make([]float32, f.dim)
		for _, w := range tokenize(s) {
			if idx, ok := f.vocab[w]; ok {
				v[idx] += 1
			}
		}
		out[i] = v
	}
	return out, nil
}

func tokenize(s string) []string {
	s = strings.ToLower(s)
	var out []string
	cur := ""
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur += string(r)
		} else {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// vocab 覆盖测试语料的关键词。
var testVocab = []string{
	"cat", "dog", "animal", "pet", "fetch",
	"python", "code", "program", "function", "language",
	"weather", "rain", "sun", "forecast",
	"food", "recipe", "cook",
}

func newTestStore(t *testing.T, path string, dim int) *VectorMemoryStore {
	t.Helper()
	embed := newFakeEmbedProvider(testVocab)
	s, err := NewVectorMemoryStore(path, embed, dim)
	if err != nil {
		t.Fatalf("NewVectorMemoryStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mkMemory(id, persona string, layer agentcontract.MemoryLayer, content string, ts time.Time, tags ...string) *agentcontract.Memory {
	return &agentcontract.Memory{
		ID:        id,
		PersonaID: persona,
		Layer:     layer,
		Content:   content,
		Tags:      tags,
		Metadata:  map[string]any{},
		CreatedAt: ts,
	}
}

// --- A1: 接口满足性 + 编译 ---
func TestVectorMemoryStore_ImplementsMemoryStore(t *testing.T) {
	var _ agentcontract.MemoryStore = (*VectorMemoryStore)(nil)
	// 运行期实例化也通过（NewVectorMemoryStore 内部已做接口断言式构造）。
	s := newTestStore(t, ":memory:", len(testVocab))
	_ = s
}

// --- A2: Save 写入含 embedding 的 Memory，Query 按余弦相似度 Top-K 返回 ---
func TestVectorMemoryStore_TopKByCosineSimilarity(t *testing.T) {
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// 三条 longterm 记忆，两条关于猫/狗（与 "cat" 查询接近），一条关于代码。
	if err := s.Save(ctx, mkMemory("m1", "p1", agentcontract.MemoryLayerLongTerm, "my cat is a lovely pet animal", base, "animal")); err != nil {
		t.Fatalf("Save m1: %v", err)
	}
	if err := s.Save(ctx, mkMemory("m2", "p1", agentcontract.MemoryLayerLongTerm, "the dog likes to fetch and play", base.Add(time.Second), "animal")); err != nil {
		t.Fatalf("Save m2: %v", err)
	}
	if err := s.Save(ctx, mkMemory("m3", "p1", agentcontract.MemoryLayerLongTerm, "python function is a reusable code block", base.Add(2*time.Second), "code")); err != nil {
		t.Fatalf("Save m3: %v", err)
	}

	// 验证 m1/m2 持久化了 embedding（非 nil）。
	got, err := s.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Query all: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("应有 3 条记忆，实际 %d", len(got))
	}
	for _, m := range got {
		if len(m.Embedding) == 0 {
			t.Fatalf("记忆 %s 应有 embedding，实际为空", m.ID)
		}
	}

	// 查询 "cat animal" —— Top-1 应为 m1（cat/animal 词完全重合）。
	top, err := s.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{
		Limit:     2,
		QueryText: "cat animal",
	})
	if err != nil {
		t.Fatalf("Query knn: %v", err)
	}
	if len(top) == 0 {
		t.Fatalf("应有检索结果，实际空")
	}
	if top[0].ID != "m1" {
		t.Fatalf("Top-1 应为 m1（cat/animal 最相似），实际 %s", top[0].ID)
	}
	// Top-2 应为 m2（dog/pet/animal）而非 m3（python/code）—— 验证相似度排序。
	if len(top) >= 2 && top[1].ID == "m3" {
		t.Fatalf("Top-2 不应为 m3（python/code 与查询无关）")
	}
}

// --- A3: Working 层跳过 embedding（nil embedding，规则检索） ---
func TestVectorMemoryStore_WorkingLayerNoEmbedding(t *testing.T) {
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// Working 层记忆：即便 store 配了 provider，也不应生成 embedding。
	m := mkMemory("w1", "p1", agentcontract.MemoryLayerWorking, "user asked about the weather forecast rain", base)
	if err := s.Save(ctx, m); err != nil {
		t.Fatalf("Save working: %v", err)
	}
	got, err := s.Query(ctx, "p1", agentcontract.MemoryLayerWorking, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Query working: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("应有 1 条 working 记忆，实际 %d", len(got))
	}
	if len(got[0].Embedding) != 0 {
		t.Fatalf("Working 层记忆 embedding 应为 nil/空，实际 len=%d", len(got[0].Embedding))
	}
	// Working 层即便带 QueryText 也走规则检索（按时间倒序）。
	if _, err := s.Query(ctx, "p1", agentcontract.MemoryLayerWorking, agentcontract.QueryOptions{
		Limit:     5,
		QueryText: "weather",
	}); err != nil {
		t.Fatalf("Working 层 Query 不应报错: %v", err)
	}
}

// --- A4: 存储契约测试（Save/Query/Delete 往返一致）---
func TestVectorMemoryStore_ContractRoundtrip(t *testing.T) {
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// Save → Query 回读 → 字段一致。
	meta := map[string]any{"source": "test", "weight": 3}
	src := &agentcontract.Memory{
		ID: "c1", PersonaID: "p1", Layer: agentcontract.MemoryLayerPreference,
		Content: "prefer python as the program language",
		Tags:    []string{"lang", "pref"}, Metadata: meta,
		CreatedAt: base,
	}
	if err := s.Save(ctx, src); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Query(ctx, "p1", agentcontract.MemoryLayerPreference, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) != 1 || got[0].ID != "c1" {
		t.Fatalf("应回读 c1，实际 %+v", got)
	}
	r := got[0]
	if r.PersonaID != src.PersonaID || r.Layer != src.Layer || r.Content != src.Content {
		t.Fatalf("核心字段不一致: %+v vs %+v", r, src)
	}
	if r.CreatedAt != src.CreatedAt {
		t.Fatalf("CreatedAt 不一致: got %v want %v", r.CreatedAt, src.CreatedAt)
	}
	if len(r.Tags) != 2 || r.Tags[0] != "lang" || r.Tags[1] != "pref" {
		t.Fatalf("Tags 不一致: %+v", r.Tags)
	}
	if r.Metadata["source"] != "test" {
		t.Fatalf("Metadata source 不一致: %+v", r.Metadata)
	}
	// JSON 解码数字为 float64。
	if w, ok := r.Metadata["weight"].(float64); !ok || w != 3 {
		t.Fatalf("Metadata weight 应为 float64(3)，实际 %+v", r.Metadata["weight"])
	}
	if len(r.Embedding) == 0 {
		t.Fatalf("Preference 层应有 embedding")
	}

	// Delete 后再 Query 应为空。
	if err := s.Delete(ctx, "c1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	after, err := s.Query(ctx, "p1", agentcontract.MemoryLayerPreference, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Query after delete: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("删除后应无记忆，实际 %d 条", len(after))
	}

	// 检索无结果返回空切片（非 nil）。
	empty, err := s.Query(ctx, "p1", agentcontract.MemoryLayerPreference, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Query empty: %v", err)
	}
	if empty == nil {
		t.Fatalf("无结果应返回空切片，实际 nil")
	}
	if len(empty) != 0 {
		t.Fatalf("无结果应长度为 0，实际 %d", len(empty))
	}
}

// --- A4 补充: Save 返回错误时不破坏已存数据 + 非法输入校验 ---
func TestVectorMemoryStore_SaveValidation(t *testing.T) {
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()

	// 非法 layer。
	if err := s.Save(ctx, &agentcontract.Memory{ID: "x", PersonaID: "p", Layer: "bogus", Content: "c", CreatedAt: time.Now()}); err == nil {
		t.Fatalf("非法 layer 应报错")
	}
	// 空 PersonaID。
	if err := s.Save(ctx, &agentcontract.Memory{ID: "x", Layer: agentcontract.MemoryLayerLongTerm, Content: "c", CreatedAt: time.Now()}); err == nil {
		t.Fatalf("空 PersonaID 应报错")
	}
	// nil Memory。
	if err := s.Save(ctx, nil); err == nil {
		t.Fatalf("nil Memory 应报错")
	}
	// 空 id Delete。
	if err := s.Delete(ctx, ""); err == nil {
		t.Fatalf("空 id Delete 应报错")
	}
}

// --- A4 补充: 向量维度不一致校验 ---
func TestVectorMemoryStore_DimensionMismatch(t *testing.T) {
	// 已冻结维度 = len(testVocab)。
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()

	// 手工塞入一条维度不一致的 embedding。
	m := mkMemory("d1", "p1", agentcontract.MemoryLayerLongTerm, "cat", time.Now())
	m.Embedding = make([]float32, len(testVocab)+1) // 多一维
	if err := s.Save(ctx, m); err == nil {
		t.Fatalf("维度不一致应报错")
	} else if !strings.Contains(err.Error(), "维度") {
		t.Fatalf("应为维度错误，实际: %v", err)
	}
}

// --- A4 补充: Tags 过滤 ---
func TestVectorMemoryStore_TagFilter(t *testing.T) {
	s := newTestStore(t, ":memory:", len(testVocab))
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	_ = s.Save(ctx, mkMemory("t1", "p1", agentcontract.MemoryLayerLongTerm, "cat animal pet", base, "animal", "furry"))
	_ = s.Save(ctx, mkMemory("t2", "p1", agentcontract.MemoryLayerLongTerm, "dog animal pet", base.Add(time.Second), "animal"))
	_ = s.Save(ctx, mkMemory("t3", "p1", agentcontract.MemoryLayerLongTerm, "python code", base.Add(2*time.Second), "code"))

	// 规则检索 + tag 过滤（Working 层强制规则路径）。
	got, err := s.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{Limit: 5, Tags: []string{"furry"}})
	if err != nil {
		t.Fatalf("Query tag: %v", err)
	}
	if len(got) != 1 || got[0].ID != "t1" {
		t.Fatalf("tag=furry 应只匹配 t1，实际 %+v", got)
	}
}

// --- A5: 跨会话恢复（临时文件 SQLite 写 → Close → 重开 → Query） ---
func TestVectorMemoryStore_CrossSessionRecovery(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mem.sqlite")
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// 会话 1：写入。
	s1, err := NewVectorMemoryStore(dbPath, newFakeEmbedProvider(testVocab), len(testVocab))
	if err != nil {
		t.Fatalf("session1 open: %v", err)
	}
	memories := []*agentcontract.Memory{
		mkMemory("r1", "p1", agentcontract.MemoryLayerLongTerm, "cat animal pet", base),
		mkMemory("r2", "p1", agentcontract.MemoryLayerLongTerm, "dog animal pet fetch", base.Add(time.Second)),
		mkMemory("r3", "p1", agentcontract.MemoryLayerPreference, "prefer python program", base),
		mkMemory("r4", "p1", agentcontract.MemoryLayerWorking, "scratch note", base),
	}
	for _, m := range memories {
		if err := s1.Save(ctx, m); err != nil {
			t.Fatalf("session1 Save %s: %v", m.ID, err)
		}
	}
	if err := s1.Close(); err != nil {
		t.Fatalf("session1 close: %v", err)
	}

	// 会话 2：重开同一文件，应能检索到（含向量检索）。
	s2, err := NewVectorMemoryStore(dbPath, newFakeEmbedProvider(testVocab), len(testVocab))
	if err != nil {
		t.Fatalf("session2 open: %v", err)
	}
	defer s2.Close()

	// 规则检索：Preference 层。
	pref, err := s2.Query(ctx, "p1", agentcontract.MemoryLayerPreference, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("session2 Query pref: %v", err)
	}
	if len(pref) != 1 || pref[0].ID != "r3" {
		t.Fatalf("跨会话应恢复 r3，实际 %+v", pref)
	}

	// 规则检索：Working 层（无 embedding）。
	work, err := s2.Query(ctx, "p1", agentcontract.MemoryLayerWorking, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("session2 Query working: %v", err)
	}
	if len(work) != 1 || work[0].ID != "r4" {
		t.Fatalf("跨会话应恢复 r4，实际 %+v", work)
	}

	// 向量检索：LongTerm 层 Top-K。
	top, err := s2.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{
		Limit:     2,
		QueryText: "cat",
	})
	if err != nil {
		t.Fatalf("session2 Query knn: %v", err)
	}
	if len(top) == 0 {
		t.Fatalf("跨会话向量检索应返回结果")
	}
	if top[0].ID != "r1" {
		t.Fatalf("跨会话 Top-1 应为 r1（cat 最相似），实际 %s", top[0].ID)
	}

	// Delete 在会话 2 也可用。
	if err := s2.Delete(ctx, "r1"); err != nil {
		t.Fatalf("session2 Delete: %v", err)
	}
	after, _ := s2.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{Limit: 5})
	for _, m := range after {
		if m.ID == "r1" {
			t.Fatalf("r1 删除后仍被检索到")
		}
	}
}

// --- A4 补充: nil EmbeddingProvider 时退化为规则检索 ---
func TestVectorMemoryStore_NilProviderDegradesGracefully(t *testing.T) {
	s, err := NewVectorMemoryStore(":memory:", nil, 0)
	if err != nil {
		t.Fatalf("NewVectorMemoryStore nil provider: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	base := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

	// 无 provider，LongTerm 层 Save 不报错，记录不带 embedding。
	if err := s.Save(ctx, mkMemory("n1", "p1", agentcontract.MemoryLayerLongTerm, "some content", base)); err != nil {
		t.Fatalf("Save no-provider: %v", err)
	}
	got, err := s.Query(ctx, "p1", agentcontract.MemoryLayerLongTerm, agentcontract.QueryOptions{Limit: 5})
	if err != nil {
		t.Fatalf("Query no-provider: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("应回读 1 条，实际 %d", len(got))
	}
	if len(got[0].Embedding) != 0 {
		t.Fatalf("无 provider 时不应有 embedding")
	}
}

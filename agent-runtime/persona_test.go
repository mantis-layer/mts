package agentruntime

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestPersona_StorageContract A4：MemoryStorage 与 SQLiteStorage 的 Persona
// 存取契约对等（Save/Get/List + NotFoundError + last-write-wins）。
func TestPersona_StorageContract(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		// 初始无 Persona：List 为空、Get 报 NotFoundError。
		all, err := s.ListPersonas(ctx)
		if err != nil || len(all) != 0 {
			t.Fatalf("ListPersonas 初始 = %v, err=%v，期望空切片", all, err)
		}
		if _, err := s.GetPersona(ctx, "ghost"); err == nil {
			t.Fatal("GetPersona(不存在) 期望 NotFoundError，得到 nil")
		} else if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("GetPersona(不存在) 期望 NotFoundError，得到 %T: %v", err, err)
		}

		// Save → Get 往返（全字段对等）。
		now := time.Now().UTC().Truncate(time.Second)
		p := &Persona{
			ID:           "p1",
			Name:         "研究员",
			Role:         "researcher",
			SystemPrompt: "你是一名严谨的研究员。",
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := s.SavePersona(ctx, p); err != nil {
			t.Fatalf("SavePersona: %v", err)
		}
		got, err := s.GetPersona(ctx, "p1")
		if err != nil {
			t.Fatalf("GetPersona: %v", err)
		}
		if got.ID != p.ID || got.Name != p.Name || got.Role != p.Role ||
			got.SystemPrompt != p.SystemPrompt {
			t.Fatalf("GetPersona 字段不符: got=%+v want=%+v", got, p)
		}
		if !got.CreatedAt.Equal(p.CreatedAt) || !got.UpdatedAt.Equal(p.UpdatedAt) {
			t.Fatalf("GetPersona 时间不符: got CreatedAt=%v UpdatedAt=%v want %v/%v",
				got.CreatedAt, got.UpdatedAt, p.CreatedAt, p.UpdatedAt)
		}

		// last-write-wins：同 ID 再写一次，字段被覆盖。
		p.Name = "高级研究员"
		p.SystemPrompt = "更新后的 prompt"
		p.UpdatedAt = now.Add(time.Hour)
		if err := s.SavePersona(ctx, p); err != nil {
			t.Fatalf("SavePersona(update): %v", err)
		}
		got2, err := s.GetPersona(ctx, "p1")
		if err != nil {
			t.Fatalf("GetPersona after update: %v", err)
		}
		if got2.Name != "高级研究员" || got2.SystemPrompt != "更新后的 prompt" ||
			!got2.UpdatedAt.Equal(now.Add(time.Hour)) {
			t.Fatalf("last-write-wins 未生效: %+v", got2)
		}
	}
}

// TestPersona_ListOrder A4：ListPersonas 按 UpdatedAt 倒序（最近修改在前）。
func TestPersona_ListOrder(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		base := time.Now().UTC().Truncate(time.Second)
		// 故意乱序写入，验证排序与写入顺序无关。
		ps := []*Persona{
			{ID: "old", Name: "旧", UpdatedAt: base, CreatedAt: base},
			{ID: "newest", Name: "最新", UpdatedAt: base.Add(2 * time.Hour), CreatedAt: base},
			{ID: "mid", Name: "中", UpdatedAt: base.Add(time.Hour), CreatedAt: base},
		}
		for _, p := range ps {
			if err := s.SavePersona(ctx, p); err != nil {
				t.Fatalf("SavePersona(%s): %v", p.ID, err)
			}
		}
		got, err := s.ListPersonas(ctx)
		if err != nil {
			t.Fatalf("ListPersonas: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("ListPersonas 数量 = %d，期望 3", len(got))
		}
		want := []string{"newest", "mid", "old"}
		for i, w := range want {
			if got[i].ID != w {
				t.Fatalf("ListPersonas 顺序错误 位置%d = %q，期望 %q（全量=%v）", i, got[i].ID, w, got)
			}
		}
	}
}

// TestPersona_CrossSessionRestore A3：跨会话恢复。
// 写入 SQLite → Close → 重开同一文件 → 读回字段一致（验证落盘而非仅内存）。
func TestPersona_CrossSessionRestore(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "persona.db")

	writeTime := time.Now().UTC().Truncate(time.Second)
	func() {
		s, err := NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("首次 NewSQLiteStorage: %v", err)
		}
		p := &Persona{
			ID:           "p-persist",
			Name:         "持久化角色",
			Role:         "assistant",
			SystemPrompt: "跨会话恢复 prompt",
			CreatedAt:    writeTime,
			UpdatedAt:    writeTime.Add(time.Minute),
		}
		if err := s.SavePersona(ctx, p); err != nil {
			t.Fatalf("SavePersona: %v", err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}()

	// 重开同一文件：模拟进程重启后的新会话。
	s2, err := NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("重开 NewSQLiteStorage: %v", err)
	}
	defer s2.Close()

	got, err := s2.GetPersona(ctx, "p-persist")
	if err != nil {
		t.Fatalf("重开后 GetPersona: %v", err)
	}
	if got.Name != "持久化角色" || got.Role != "assistant" || got.SystemPrompt != "跨会话恢复 prompt" {
		t.Fatalf("跨会话字段不符: %+v", got)
	}
	if !got.CreatedAt.Equal(writeTime) || !got.UpdatedAt.Equal(writeTime.Add(time.Minute)) {
		t.Fatalf("跨会话时间不符: CreatedAt=%v UpdatedAt=%v", got.CreatedAt, got.UpdatedAt)
	}

	// List 也应在新会话可见。
	all, err := s2.ListPersonas(ctx)
	if err != nil || len(all) != 1 || all[0].ID != "p-persist" {
		t.Fatalf("重开后 ListPersonas = %v, err=%v，期望单条 p-persist", all, err)
	}
}

// TestSubmitTask_PersonaBinding A2：SubmitTask 据 PersonaID 加载 Persona。
// - PersonaID 为空：向后兼容，成功（无 Persona）。
// - PersonaID 命中：成功。
// - PersonaID 不存在：返回 NotFoundError，不静默 nil。
func TestSubmitTask_PersonaBinding(t *testing.T) {
	ctx := context.Background()

	// 场景一：PersonaID 为空（向后兼容）。
	{
		rt, _ := NewRuntime(NewMemoryStorage(), Budget{})
		rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
		run, err := rt.SubmitTask(ctx, &Task{ID: "t-nopersona", Pattern: "mock", Input: "x"})
		if err != nil {
			t.Fatalf("空 PersonaID 应成功（向后兼容）: %v", err)
		}
		if run == nil || run.TaskID != "t-nopersona" {
			t.Fatalf("空 PersonaID 返回异常 run: %+v", run)
		}
	}

	// 场景二：PersonaID 命中。
	{
		mem := NewMemoryStorage()
		now := time.Now().UTC().Truncate(time.Second)
		if err := mem.SavePersona(ctx, &Persona{
			ID: "p1", Name: "研究员", Role: "researcher",
			SystemPrompt: "sp", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePersona: %v", err)
		}
		rt, _ := NewRuntime(mem, Budget{})
		rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
		run, err := rt.SubmitTask(ctx, &Task{
			ID: "t-withpersona", Pattern: "mock", Input: "x", PersonaID: "p1",
		})
		if err != nil {
			t.Fatalf("PersonaID 命中应成功: %v", err)
		}
		if run.TaskID != "t-withpersona" {
			t.Fatalf("run.TaskID = %q", run.TaskID)
		}
	}

	// 场景三：PersonaID 不存在 → NotFoundError（不静默 nil）。
	{
		rt, _ := NewRuntime(NewMemoryStorage(), Budget{})
		rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
		_, err := rt.SubmitTask(ctx, &Task{
			ID: "t-ghost", Pattern: "mock", Input: "x", PersonaID: "no-such-persona",
		})
		if err == nil {
			t.Fatal("PersonaID 不存在应报错，得到 nil")
		}
		var nfe *NotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("期望 NotFoundError，得到 %T: %v", err, err)
		}
		if nfe.ID != "no-such-persona" {
			t.Fatalf("NotFoundError.ID = %q，期望 no-such-persona", nfe.ID)
		}
	}
}

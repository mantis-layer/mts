package agentruntime

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// newTestStorage 返回内存与 SQLite 两种实现（E9：双实现契约测试）。
func newTestStorage(t *testing.T) []Storage {
	t.Helper()
	mem := NewMemoryStorage()
	db := filepath.Join(t.TempDir(), "test.db")
	sql, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = sql.Close() })
	return []Storage{mem, sql}
}

func TestStorage_RoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		task := &Task{ID: "t1", Name: "任务", Pattern: "tool_loop", Input: "hello", CreatedAt: time.Now()}
		if err := s.SaveTask(ctx, task); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		got, err := s.GetTask(ctx, "t1")
		if err != nil || got.Input != "hello" || got.Pattern != "tool_loop" {
			t.Fatalf("GetTask = %v, %v", got, err)
		}

		run := &TaskRun{ID: "r1", TaskID: "t1", Pattern: "tool_loop", State: RunStateCreated, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := s.CreateRun(ctx, run); err != nil {
			t.Fatalf("CreateRun: %v", err)
		}
		run.State = RunStateRunning
		run.Iterations = 2
		if err := s.UpdateRun(ctx, run); err != nil {
			t.Fatalf("UpdateRun: %v", err)
		}
		r2, err := s.GetRun(ctx, "r1")
		if err != nil || r2.State != RunStateRunning || r2.Iterations != 2 {
			t.Fatalf("GetRun = %v, %v", r2, err)
		}

		ev := &RuntimeEvent{ID: "e1", TaskRunID: "r1", Kind: EventTaskRunStarted, Timestamp: time.Now()}
		if err := s.AddEvent(ctx, ev); err != nil {
			t.Fatalf("AddEvent: %v", err)
		}
		evs, err := s.Events(ctx, "r1")
		if err != nil || len(evs) != 1 || evs[0].Kind != EventTaskRunStarted {
			t.Fatalf("Events = %v, %v", evs, err)
		}

		a := &Artifact{ID: "a1", TaskRunID: "r1", Name: "report", Type: ArtifactJSON, Content: `{"ok":true}`, CreatedAt: time.Now()}
		if err := s.AddArtifact(ctx, a); err != nil {
			t.Fatalf("AddArtifact: %v", err)
		}
		arts, err := s.Artifacts(ctx, "r1")
		if err != nil || len(arts) != 1 || arts[0].Content != `{"ok":true}` {
			t.Fatalf("Artifacts = %v, %v", arts, err)
		}
		if err := s.AddEvidence(ctx, &Evidence{ArtifactID: "a1", Source: "file.json", Quote: "x"}); err != nil {
			t.Fatalf("AddEvidence: %v", err)
		}
		evs2, err := s.Evidence(ctx, "a1")
		if err != nil || len(evs2) != 1 {
			t.Fatalf("Evidence = %v, %v", evs2, err)
		}
	}
}

func TestStorage_NotFound(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		if _, err := s.GetTask(ctx, "nope"); err == nil {
			t.Fatal("期望 NotFoundError，得到 nil")
		} else if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("期望 NotFoundError，得到 %T: %v", err, err)
		}
		if _, err := s.GetRun(ctx, "nope"); err == nil {
			t.Fatal("期望 NotFoundError，得到 nil")
		}
	}
}

// TestStorage_Persona A4：Persona 存取契约（双实现对等）。
// 覆盖 Save/Get/List/NotFoundError；跨会话恢复与排序细节见 persona_test.go。
func TestStorage_Persona(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		now := time.Now().UTC().Truncate(time.Second)
		if err := s.SavePersona(ctx, &Persona{
			ID: "sp1", Name: "存储测试", Role: "r",
			SystemPrompt: "prompt", CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("SavePersona: %v", err)
		}
		got, err := s.GetPersona(ctx, "sp1")
		if err != nil || got.Name != "存储测试" || got.SystemPrompt != "prompt" {
			t.Fatalf("GetPersona = %+v, %v", got, err)
		}
		all, err := s.ListPersonas(ctx)
		if err != nil || len(all) != 1 || all[0].ID != "sp1" {
			t.Fatalf("ListPersonas = %+v, %v", all, err)
		}
		// NotFoundError 契约。
		if _, err := s.GetPersona(ctx, "missing"); err == nil {
			t.Fatal("GetPersona(不存在) 期望 NotFoundError")
		} else if _, ok := err.(*NotFoundError); !ok {
			t.Fatalf("期望 NotFoundError，得到 %T: %v", err, err)
		}
	}
}

// TestSQLite_MigrationOldSchema S1 回归：旧 schema 库（无 progress/task_input 列）
// 打开后自动迁移 + 回填，GetRun 不因 NULL 崩溃。
func TestSQLite_MigrationOldSchema(t *testing.T) {
	db := filepath.Join(t.TempDir(), "old.db")
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE runs (
		id TEXT PRIMARY KEY, task_id TEXT, pattern TEXT, state TEXT, iterations INTEGER,
		tool_calls INTEGER, usage_json TEXT, summary TEXT, input TEXT, result_json TEXT,
		error TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO runs (id, task_id, pattern, state, iterations, tool_calls, usage_json, summary, input, created_at, updated_at)
		VALUES ('r_old', 't', 'tool_loop', 'completed', 1, 0, '{}', 's', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	s, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage(旧库): %v", err)
	}
	defer s.Close()
	r, err := s.GetRun(context.Background(), "r_old")
	if err != nil {
		t.Fatalf("GetRun(旧行): %v", err)
	}
	if r.Progress != "" || r.TaskInput != "" {
		t.Fatalf("迁移后 Progress=%q TaskInput=%q，期望空串", r.Progress, r.TaskInput)
	}
}

// TestStorage_TaskPersonaIDRoundTrip 验证 Task.PersonaID 在双实现（含 SQLite 落盘）往返不丢。
// 回归保护：曾出现"加了字段但 tasks 表未持久化 persona_id"的缺口。
func TestStorage_TaskPersonaIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	for _, s := range newTestStorage(t) {
		task := &Task{ID: "t-pid", Name: "persona task", Pattern: "tool_loop",
			Input: "hi", PersonaID: "persona-7", CreatedAt: time.Now()}
		if err := s.SaveTask(ctx, task); err != nil {
			t.Fatalf("SaveTask: %v", err)
		}
		got, err := s.GetTask(ctx, "t-pid")
		if err != nil {
			t.Fatalf("GetTask: %v", err)
		}
		if got.PersonaID != "persona-7" {
			t.Fatalf("PersonaID 往返丢失: got %q, want persona-7", got.PersonaID)
		}

		// 空 PersonaID（向后兼容 v0.1 无 Persona 的 Task）也必须往返一致。
		task2 := &Task{ID: "t-nopid", Name: "legacy", Pattern: "tool_loop",
			Input: "hi", CreatedAt: time.Now()}
		if err := s.SaveTask(ctx, task2); err != nil {
			t.Fatalf("SaveTask(空 PersonaID): %v", err)
		}
		got2, err := s.GetTask(ctx, "t-nopid")
		if err != nil {
			t.Fatalf("GetTask(空): %v", err)
		}
		if got2.PersonaID != "" {
			t.Fatalf("空 PersonaID 往返不一致: got %q, want empty", got2.PersonaID)
		}
	}
}

// TestSQLite_TaskPersonaIDMigration 旧 tasks 表（无 persona_id 列）打开后自动迁移 + 回填，
// SaveTask/GetTask 不因 NULL 崩溃。
func TestSQLite_TaskPersonaIDMigration(t *testing.T) {
	db := filepath.Join(t.TempDir(), "old-tasks.db")
	raw, err := sql.Open("sqlite", db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE tasks (
		id TEXT PRIMARY KEY, name TEXT, pattern TEXT, input TEXT, created_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`INSERT INTO tasks (id, name, pattern, input, created_at)
		VALUES ('t_old', 'n', 'tool_loop', 'x', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = raw.Close()

	s, err := NewSQLiteStorage(db)
	if err != nil {
		t.Fatalf("NewSQLiteStorage(旧 tasks 库): %v", err)
	}
	defer s.Close()
	got, err := s.GetTask(context.Background(), "t_old")
	if err != nil {
		t.Fatalf("GetTask(旧行迁移后): %v", err)
	}
	if got.PersonaID != "" {
		t.Fatalf("旧行迁移后 PersonaID=%q，期望空串回填", got.PersonaID)
	}
}

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

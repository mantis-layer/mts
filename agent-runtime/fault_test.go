package agentruntime

import (
	"context"
	"fmt"
	"testing"
)

// failingStorage 在指定操作上注入失败（Fault Injection）。
type failingStorage struct {
	Storage
	failOp    string // 失败操作名
	failTimes int    // 剩余失败次数
	events    []string
}

func (f *failingStorage) UpdateRunIf(ctx context.Context, run *TaskRun, from RunState) (bool, error) {
	f.events = append(f.events, "update_run_if")
	if f.failOp == "update_run_if" && f.failTimes > 0 {
		f.failTimes--
		return false, fmt.Errorf("注入的 storage 写失败")
	}
	return f.Storage.UpdateRunIf(ctx, run, from)
}

func (f *failingStorage) AddArtifactsEvidence(ctx context.Context, arts []Artifact, evs []Evidence) error {
	f.events = append(f.events, "add_artifacts_evidence")
	if f.failOp == "add_artifacts_evidence" && f.failTimes > 0 {
		f.failTimes--
		return fmt.Errorf("注入的 artifact 落库失败")
	}
	return f.Storage.AddArtifactsEvidence(ctx, arts, evs)
}

// TestRuntime_StorageWriteFailure S2：checkpoint 写失败 → run failed 且不崩溃。
func TestRuntime_StorageWriteFailure(t *testing.T) {
	base := NewMemoryStorage()
	fs := &failingStorage{Storage: base, failOp: "update_run_if", failTimes: 1}
	rt, err := NewRuntime(fs, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
		{Iterations: 1}, {Done: true, Iterations: 1},
	}})
	ctx := context.Background()
	run, err := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	if err != nil {
		t.Fatalf("SubmitTask: %v", err)
	}
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 写失败 → failed（不崩溃、不静默完成）
	if final.State != RunStateFailed {
		t.Fatalf("终态 = %s，期望 failed（storage 写失败）", final.State)
	}
	if final.Error == "" {
		t.Fatal("Error 应含失败原因")
	}
	// 事件流完整（至少 created/started）
	evs, err := rt.Events(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) < 2 {
		t.Fatalf("事件过少: %d", len(evs))
	}
}

// TestRuntime_ArtifactWriteFailure S2：Artifact 落库失败 → failed 且不留脏数据。
func TestRuntime_ArtifactWriteFailure(t *testing.T) {
	base := NewMemoryStorage()
	fs := &failingStorage{Storage: base, failOp: "add_artifacts_evidence", failTimes: 1}
	rt, err := NewRuntime(fs, Budget{})
	if err != nil {
		t.Fatal(err)
	}
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{
		Done:      true,
		Artifacts: []Artifact{{Name: "report", Type: ArtifactText, Content: "r"}},
		Evidence:  []Evidence{{ArtifactID: "report", Source: "s", Quote: "q"}},
	}}})
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateFailed {
		t.Fatalf("终态 = %s，期望 failed", final.State)
	}
	// 无孤立 Artifact（事务原子性）
	arts, _ := base.Artifacts(ctx, run.ID)
	if len(arts) != 0 {
		t.Fatalf("失败后 Artifacts = %v（应无孤立数据）", arts)
	}
}

package agentruntime

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// ---- mock Pattern ----

type mockPattern struct {
	name  string
	steps []StepResult
	err   error
}

func (p *mockPattern) Name() string { return p.name }
func (p *mockPattern) Execute(_ context.Context, _ *TaskRun) (*StepResult, error) {
	if p.err != nil {
		return nil, p.err
	}
	if len(p.steps) == 0 {
		return &StepResult{Done: true}, nil
	}
	s := p.steps[0]
	p.steps = p.steps[1:]
	return &s, nil
}

// blockingPattern 阻塞直到 ctx 取消（用于取消测试）。
type blockingPattern struct{}

func (blockingPattern) Name() string { return "blocking" }
func (blockingPattern) Execute(ctx context.Context, _ *TaskRun) (*StepResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func newTestRuntime(t *testing.T, s Storage, b Budget) *Runtime {
	t.Helper()
	rt, err := NewRuntime(s, b)
	if err != nil {
		t.Fatalf("NewRuntime: %v", err)
	}
	return rt
}

// ---- 并发安全（review blocking 修复验证） ----

func TestRuntime_ConcurrentRun(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		// 3 步才 Done 的 pattern：若双 Run 并发执行，Iterations 会翻倍
		rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
			{Iterations: 1}, {Iterations: 1}, {Done: true, Iterations: 1},
		}})
		ctx := context.Background()
		run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, _ = rt.Run(ctx, run.ID)
			}()
		}
		wg.Wait()

		final, err := rt.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State != RunStateCompleted {
			t.Fatalf("终态 = %s", final.State)
		}
		if final.Iterations != 3 {
			t.Fatalf("Iterations = %d，期望 3（并发重复执行会翻倍）", final.Iterations)
		}
	}
}

func TestRuntime_CancelAPIWhileRunning(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		if err := rt.RegisterPattern(blockingPattern{}); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "blocking", Input: "x"})

		done := make(chan struct{})
		go func() {
			defer close(done)
			_, _ = rt.Run(ctx, run.ID)
		}()
		time.Sleep(50 * time.Millisecond) // 确保进入 running

		// Cancel API 触发执行中 Run 的取消
		cancelled, err := rt.Cancel(ctx, run.ID)
		if err != nil {
			t.Fatalf("Cancel: %v", err)
		}
		<-done

		final, err := rt.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State != RunStateCancelled {
			t.Fatalf("Cancel API 后终态 = %s，期望 cancelled（Cancel 返回 %s）", final.State, cancelled.State)
		}
	}
}

func TestRuntime_ConcurrentHumanInput(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
		{NeedHuman: true},
		{Done: true, Output: "完成"},
	}})
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	if _, err := rt.Run(ctx, run.ID); err != nil {
		t.Fatalf("Run: %v", err)
	}

	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = rt.SubmitHumanInput(ctx, run.ID, "输入")
		}(i)
	}
	wg.Wait()

	final, err := rt.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.State != RunStateCompleted {
		t.Fatalf("终态 = %s，期望 completed", final.State)
	}
}

func TestRuntime_EvaluatorFail(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
	rt.RegisterEvaluator(&SchemaEvaluator{ArtifactName: "缺失"})
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateFailed {
		t.Fatalf("Evaluator 未通过时终态 = %s，期望 failed", final.State)
	}
}

func TestSQLite_UpdateRunNotFound(t *testing.T) {
	s := mustSQLite(t, t.TempDir()+"/u.db")
	defer s.Close()
	err := s.UpdateRun(context.Background(), &TaskRun{ID: "nope"})
	if err == nil {
		t.Fatal("UpdateRun 不存在 run 应报 NotFoundError")
	}
	if _, ok := err.(*NotFoundError); !ok {
		t.Fatalf("期望 NotFoundError，得到 %T: %v", err, err)
	}
}

// ---- 状态机生命周期（E1） ----

func TestRuntime_StateMachineLifecycle(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		if err := rt.RegisterPattern(&mockPattern{name: "mock"}); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		run, err := rt.SubmitTask(ctx, &Task{ID: "t1", Name: "任务", Pattern: "mock", Input: "hello"})
		if err != nil {
			t.Fatalf("SubmitTask: %v", err)
		}
		if run.State != RunStateCreated {
			t.Fatalf("初始状态 = %s，期望 created", run.State)
		}
		final, err := rt.Run(ctx, run.ID)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if final.State != RunStateCompleted {
			t.Fatalf("终态 = %s，期望 completed", final.State)
		}
		evs, _ := rt.Events(ctx, run.ID)
		kinds := map[EventKind]bool{}
		for _, e := range evs {
			kinds[e.Kind] = true
		}
		for _, want := range []EventKind{EventTaskRunCreated, EventTaskRunStarted, EventTaskRunCompleted} {
			if !kinds[want] {
				t.Fatalf("缺少事件 %s，got %v", want, kinds)
			}
		}
		// 幂等：终态重复 Run 直接返回
		if _, err := rt.Run(ctx, run.ID); err != nil {
			t.Fatalf("终态重复 Run 应幂等: %v", err)
		}
	}
}

// ---- 预算耗尽（E3） ----

func TestRuntime_BudgetExceeded(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{MaxIterations: 2})
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
		{Iterations: 1}, {Iterations: 1}, {Iterations: 1}, // 第三轮触发预算
	}})
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateFailed {
		t.Fatalf("终态 = %s，期望 failed", final.State)
	}
	evs, _ := rt.Events(ctx, run.ID)
	found := false
	for _, e := range evs {
		if e.Kind == EventBudgetExceeded {
			found = true
		}
	}
	if !found {
		t.Fatal("缺少 EventBudgetExceeded")
	}
}

// ---- 取消（E4） ----

func TestRuntime_Cancel(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		if err := rt.RegisterPattern(blockingPattern{}); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "blocking", Input: "x"})

		ctx2, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			_, err := rt.Run(ctx2, run.ID)
			done <- err
		}()
		time.Sleep(50 * time.Millisecond)
		cancel()
		<-done

		final, err := rt.GetRun(ctx, run.ID)
		if err != nil {
			t.Fatal(err)
		}
		if final.State != RunStateCancelled {
			t.Fatalf("终态 = %s，期望 cancelled", final.State)
		}

		// created 状态直接 Cancel
		run2, _ := rt.SubmitTask(ctx, &Task{ID: "t2", Pattern: "blocking", Input: "x"})
		c2, _ := rt.Cancel(ctx, run2.ID)
		if c2.State != RunStateCancelled {
			t.Fatalf("created 取消后状态 = %s", c2.State)
		}
	}
}

// TestRuntime_CancelStartupRace 覆盖 Cancel 与 Run 启动瞬间的交错。
// 不变式：只要 Cancel 返回 cancelled，最终终态必须收敛为 cancelled
// （防止旧 bug：注册窗口内取消被覆盖成 completed）。
func TestRuntime_CancelStartupRace(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
			{Iterations: 1}, {Iterations: 1}, {Done: true, Iterations: 1},
		}})
		ctx := context.Background()
		for i := 0; i < 20; i++ {
			run, err := rt.SubmitTask(ctx, &Task{ID: "t" + itoa(i), Pattern: "mock", Input: "x"})
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			var mu sync.Mutex
			var cancelState RunState
			wg.Add(2)
			go func() { defer wg.Done(); _, _ = rt.Run(ctx, run.ID) }()
			go func() {
				defer wg.Done()
				c, err := rt.Cancel(ctx, run.ID)
				if err == nil {
					mu.Lock()
					cancelState = c.State
					mu.Unlock()
				}
			}()
			wg.Wait()

			final, err := rt.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			cs := cancelState
			mu.Unlock()
			if final.State == RunStateRunning {
				t.Fatalf("迭代 %d: 终态 = %s，Run 未收敛", i, final.State)
			}
			if final.State != RunStateCancelled && final.State != RunStateCompleted {
				t.Fatalf("迭代 %d: 终态 = %s（cs=%s），期望 cancelled 或 completed", i, final.State, cs)
			}
			// 不变式：Cancel 返回 cancelled → 终态必为 cancelled（防止取消被覆盖）
			if cs == RunStateCancelled && final.State != RunStateCancelled {
				t.Fatalf("迭代 %d: Cancel 返回 cancelled 但终态 = %s（取消被覆盖）", i, final.State)
			}
		}
	}
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

// ---- HITL（E8） ----

func TestRuntime_HITL(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	p := &mockPattern{name: "mock", steps: []StepResult{
		{NeedHuman: true, HumanPrompt: "确认继续？"},
		{Done: true, Output: "最终结果"},
	}}
	rt.RegisterPattern(p)
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})

	waiting, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if waiting.State != RunStateWaiting {
		t.Fatalf("状态 = %s，期望 waiting", waiting.State)
	}
	evs, _ := rt.Events(ctx, run.ID)
	hasReq := false
	for _, e := range evs {
		if e.Kind == EventHumanInputRequested {
			hasReq = true
		}
	}
	if !hasReq {
		t.Fatal("缺少 EventHumanInputRequested")
	}

	// waiting 状态直接 Run 应报错
	if _, err := rt.Run(ctx, run.ID); err == nil {
		t.Fatal("waiting 状态 Run 应报错")
	}

	final, err := rt.SubmitHumanInput(ctx, run.ID, "确认")
	if err != nil {
		t.Fatalf("SubmitHumanInput: %v", err)
	}
	if final.State != RunStateCompleted || final.Result == nil || final.Result.Summary != "最终结果" {
		t.Fatalf("HITL 后终态 = %+v", final)
	}

	// 终态 SubmitHumanInput → StateError
	if _, err := rt.SubmitHumanInput(ctx, run.ID, "再输入"); err == nil {
		t.Fatal("终态 SubmitHumanInput 应报错")
	}
}

// ---- Evaluator（E7） ----

func TestRuntime_Evaluators(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
	rt.RegisterEvaluator(&SchemaEvaluator{ArtifactName: "report"})
	rt.RegisterEvaluator(&EvidenceCoverageEvaluator{Required: 1})

	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	art, err := rt.AddArtifact(ctx, run.ID, "report", ArtifactJSON, `{"ok":true}`)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.AddEvidence(ctx, art.ID, "source", "quote"); err != nil {
		t.Fatal(err)
	}
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateCompleted {
		t.Fatalf("终态 = %s", final.State)
	}
	evs, _ := rt.Events(ctx, run.ID)
	count := 0
	for _, e := range evs {
		if e.Kind == EventEvaluatorResult {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("EvaluatorResult 事件 = %d，期望 2", count)
	}
}

// ---- 非法状态跳转 / 未知 Pattern（E9） ----

func TestRuntime_IllegalTransitionAndUnknownPattern(t *testing.T) {
	if CanTransition(RunStateCompleted, RunStateRunning) {
		t.Fatal("completed → running 应非法")
	}
	if CanTransition(RunStateCreated, RunStateWaiting) {
		t.Fatal("created → waiting 应非法")
	}
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	ctx := context.Background()
	if _, err := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "不存在", Input: "x"}); err == nil {
		t.Fatal("未知 Pattern 应报错")
	}
}

// ---- 恢复 / Checkpoint（E5/E6） ----

func TestRuntime_Recovery(t *testing.T) {
	db := t.TempDir() + "/recovery.db"
	ctx := context.Background()

	rt1 := newTestRuntime(t, mustSQLite(t, db), Budget{})
	rt1.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{
		{NeedHuman: true, HumanPrompt: "请确认"},
		{Done: true, Output: "完成"},
	}})
	run, _ := rt1.SubmitTask(ctx, &Task{ID: "t1", Pattern: "mock", Input: "x"})
	waiting, err := rt1.Run(ctx, run.ID)
	if err != nil || waiting.State != RunStateWaiting {
		t.Fatalf("第一次运行 = %v, %v", waiting, err)
	}
	if err := rt1.Close(); err != nil {
		t.Fatal(err)
	}

	// 新进程恢复：重新打开同一 DB，状态与事件完整（S9 / E6）
	rt2 := newTestRuntime(t, mustSQLite(t, db), Budget{})
	rt2.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true, Output: "完成"}}})
	recovered, err := rt2.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("恢复 GetRun: %v", err)
	}
	if recovered.State != RunStateWaiting {
		t.Fatalf("恢复后状态 = %s，期望 waiting", recovered.State)
	}
	evs, _ := rt2.Events(ctx, run.ID)
	if len(evs) == 0 {
		t.Fatal("恢复后事件为空")
	}
	final, err := rt2.SubmitHumanInput(ctx, run.ID, "继续")
	if err != nil || final.State != RunStateCompleted {
		t.Fatalf("恢复后继续 = %v, %v", final, err)
	}
	// 恢复后 Result 与 summary 完整
	if final.Result == nil || final.Result.Summary != "完成" {
		t.Fatalf("恢复后 Result = %+v", final.Result)
	}
}

func mustSQLite(t *testing.T, path string) *SQLiteStorage {
	t.Helper()
	s, err := NewSQLiteStorage(path)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	return s
}

// ---- ToolLoopPattern（E2） ----

type echoTool struct{}

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "回显消息" }
func (echoTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"msg": map[string]any{"type": "string"}},
		"required":   []any{"msg"},
	}
}
func (echoTool) Execute(_ context.Context, input map[string]any) (map[string]any, error) {
	return map[string]any{"echo": input["msg"]}, nil
}

type mockModel struct {
	mu      sync.Mutex
	streams [][]agentmodel.StreamEvent
	calls   int
}

func (m *mockModel) Complete(_ context.Context, _ agentmodel.Request) (agentmodel.Response, error) {
	return agentmodel.Response{}, nil
}

func (m *mockModel) Stream(_ context.Context, _ agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var seq []agentmodel.StreamEvent
	if m.calls < len(m.streams) {
		seq = m.streams[m.calls]
	}
	m.calls++
	ch := make(chan agentmodel.StreamEvent, len(seq))
	for _, e := range seq {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func TestToolLoopPattern(t *testing.T) {
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		{ // 第一轮：请求调用 echo
			{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "tc1", Name: "echo", Arguments: `{"msg":"hi"}`}},
			{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}},
			{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
		},
		{ // 第二轮：最终文本
			{Kind: agentmodel.StreamEventDelta, Delta: "回显完成"},
			{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 4, CompletionTokens: 2, TotalTokens: 6}},
			{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop},
		},
	}}
	reg := agentcore.NewRegistry()
	if err := reg.Register(echoTool{}); err != nil {
		t.Fatal(err)
	}
	agent := agentcore.New(m, reg, agentcore.Options{})
	pat := NewToolLoopPattern(agent)

	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	if err := rt.RegisterPattern(pat); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "tool_loop", Input: "请回显 hi"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateCompleted {
		t.Fatalf("终态 = %s", final.State)
	}
	if final.ToolCalls != 1 {
		t.Fatalf("ToolCalls = %d，期望 1", final.ToolCalls)
	}
	if final.Result == nil || final.Result.Summary != "回显完成" {
		t.Fatalf("Result = %+v", final.Result)
	}
	if final.Usage.TotalTokens <= 0 {
		t.Fatal("Usage 未累计")
	}
}

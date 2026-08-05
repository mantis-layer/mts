package agentruntime

import (
	"context"
	"fmt"
	"sync"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Runtime 是 Task Runtime 入口：提交 Task、创建 TaskRun、执行、暂停/恢复、
// 取消、人工输入、事件与 Artifact 持久化（FR-004）。
type Runtime struct {
	storage    Storage
	patterns   map[string]Pattern
	evaluators map[string]Evaluator
	budget     Budget

	mu     sync.Mutex
	nextID int
}

// NewRuntime 构造 Runtime。storage 不能为 nil。
func NewRuntime(storage Storage, budget Budget) (*Runtime, error) {
	if storage == nil {
		return nil, fmt.Errorf("agentruntime: storage 不能为 nil")
	}
	return &Runtime{
		storage:    storage,
		patterns:   make(map[string]Pattern),
		evaluators: make(map[string]Evaluator),
		budget:     budget,
	}, nil
}

// RegisterPattern 注册 Pattern（FR-005 Host）。
func (rt *Runtime) RegisterPattern(p Pattern) error {
	if p == nil || p.Name() == "" {
		return fmt.Errorf("agentruntime: 无效 Pattern")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if _, ok := rt.patterns[p.Name()]; ok {
		return fmt.Errorf("agentruntime: Pattern %q 已注册", p.Name())
	}
	rt.patterns[p.Name()] = p
	return nil
}

// RegisterEvaluator 注册 Evaluator。
func (rt *Runtime) RegisterEvaluator(e Evaluator) error {
	if e == nil || e.Name() == "" {
		return fmt.Errorf("agentruntime: 无效 Evaluator")
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if _, ok := rt.evaluators[e.Name()]; ok {
		return fmt.Errorf("agentruntime: Evaluator %q 已注册", e.Name())
	}
	rt.evaluators[e.Name()] = e
	return nil
}

// SubmitTask 创建 Task 与 TaskRun（初始状态 created）。
func (rt *Runtime) SubmitTask(ctx context.Context, task *Task) (*TaskRun, error) {
	if task.ID == "" {
		return nil, fmt.Errorf("agentruntime: task ID 不能为空")
	}
	if _, ok := rt.getPattern(task.Pattern); !ok {
		return nil, fmt.Errorf("agentruntime: Pattern %q 未注册", task.Pattern)
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Now()
	}
	run := &TaskRun{
		ID:        newID("run"),
		TaskID:    task.ID,
		Pattern:   task.Pattern,
		State:     RunStateCreated,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := rt.storage.SaveTask(ctx, task); err != nil {
		return nil, err
	}
	if err := rt.storage.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	if err := rt.addEvent(ctx, run, EventTaskRunCreated, map[string]any{"task_id": task.ID}); err != nil {
		return nil, err
	}
	return run, nil
}

// Run 执行 TaskRun 直到终态（completed/failed/cancelled）或进入 waiting。
// 幂等：已终态直接返回；waiting 需要 SubmitHumanInput 后重新 Run。
func (rt *Runtime) Run(ctx context.Context, runID string) (*TaskRun, error) {
	run, err := rt.storage.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	switch run.State {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return run, nil // 幂等
	case RunStateWaiting:
		return run, fmt.Errorf("agentruntime: run %s 处于 waiting，需 SubmitHumanInput 后重试", runID)
	}

	if run.State != RunStateRunning {
		if err := rt.transition(ctx, run, RunStateRunning); err != nil {
			return nil, err
		}
		if err := rt.addEvent(ctx, run, EventTaskRunStarted, nil); err != nil {
			return nil, err
		}
	}

	pattern, ok := rt.getPattern(run.Pattern)
	if !ok {
		_ = rt.fail(ctx, run, fmt.Errorf("Pattern 未注册"))
		return rt.storage.GetRun(ctx, runID)
	}

	for {
		if err := ctx.Err(); err != nil {
			_ = rt.cancelRun(ctx, run, "context cancelled")
			return rt.storage.GetRun(ctx, runID)
		}
		if rt.budget.Exceeded(run.Iterations, run.ToolCalls) {
			_ = rt.addEvent(ctx, run, EventBudgetExceeded, map[string]any{
				"iterations": run.Iterations, "tool_calls": run.ToolCalls,
			})
			_ = rt.fail(ctx, run, fmt.Errorf("预算耗尽（iterations=%d tool_calls=%d）", run.Iterations, run.ToolCalls))
			return rt.storage.GetRun(ctx, runID)
		}

		step, err := pattern.Execute(ctx, run)
		if err != nil {
			if ctx.Err() != nil {
				// 取消优先于失败：执行中被取消 → cancelled
				_ = rt.cancelRun(ctx, run, "context cancelled")
			} else {
				_ = rt.fail(ctx, run, err)
			}
			return rt.storage.GetRun(ctx, runID)
		}
		run.Iterations += step.Iterations
		run.ToolCalls += step.ToolCalls
		mergeUsage(&run.Usage, step.Usage)
		run.UpdatedAt = time.Now()

		if step.NeedHuman {
			if err := rt.transition(ctx, run, RunStateWaiting); err != nil {
				return nil, err
			}
			_ = rt.addEvent(ctx, run, EventHumanInputRequested, map[string]any{"prompt": step.HumanPrompt})
			// Checkpoint：持久化等待状态
			if err := rt.storage.UpdateRun(ctx, run); err != nil {
				return nil, err
			}
			_ = rt.addEvent(ctx, run, EventCheckpointSaved, nil)
			return run, nil
		}

		if step.Output != "" {
			run.SummaryAppend(step.Output)
		}
		if err := rt.storage.UpdateRun(ctx, run); err != nil {
			_ = rt.fail(ctx, run, err)
			return rt.storage.GetRun(ctx, runID)
		}
		_ = rt.addEvent(ctx, run, EventCheckpointSaved, nil)

		if step.Done {
			break
		}
	}

	// 完成 + Evaluator 验收
	arts, err := rt.storage.Artifacts(ctx, run.ID)
	if err != nil {
		_ = rt.fail(ctx, run, err)
		return rt.storage.GetRun(ctx, runID)
	}
	run.Result = &TaskResult{
		Summary:    run.Summary,
		Usage:      run.Usage,
		ToolCalls:  run.ToolCalls,
		Iterations: run.Iterations,
		Artifacts:  arts,
	}
	if err := rt.runEvaluators(ctx, run); err != nil {
		_ = rt.fail(ctx, run, err)
		return rt.storage.GetRun(ctx, runID)
	}
	if err := rt.transition(ctx, run, RunStateCompleted); err != nil {
		return nil, err
	}
	if err := rt.storage.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	_ = rt.addEvent(ctx, run, EventTaskRunCompleted, map[string]any{"summary": run.Result.Summary})
	return rt.storage.GetRun(ctx, runID)
}

// SubmitHumanInput 向 waiting 的 TaskRun 提供人工输入（FR-004 HITL / S6 前置）。
func (rt *Runtime) SubmitHumanInput(ctx context.Context, runID, input string) (*TaskRun, error) {
	run, err := rt.storage.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.State != RunStateWaiting {
		return nil, &StateError{From: run.State, To: RunStateRunning, Op: "submit human input"}
	}
	if input == "" {
		return nil, fmt.Errorf("agentruntime: 人工输入不能为空")
	}
	run.Input = input
	if err := rt.storage.UpdateRun(ctx, run); err != nil {
		return nil, err
	}
	_ = rt.addEvent(ctx, run, EventHumanInputReceived, map[string]any{"run_id": runID})
	if err := rt.transition(ctx, run, RunStateRunning); err != nil {
		return nil, err
	}
	// 进入 running 并继续执行（阻塞直到终态或再次 waiting）
	return rt.Run(ctx, runID)
}

// Cancel 取消运行（终态幂等）。
func (rt *Runtime) Cancel(ctx context.Context, runID string) (*TaskRun, error) {
	run, err := rt.storage.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	switch run.State {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return run, nil
	}
	_ = rt.cancelRun(ctx, run, "user cancelled")
	return rt.storage.GetRun(ctx, runID)
}

// GetRun / GetTask 查询。
func (rt *Runtime) GetRun(ctx context.Context, runID string) (*TaskRun, error) {
	return rt.storage.GetRun(ctx, runID)
}
func (rt *Runtime) GetTask(ctx context.Context, taskID string) (*Task, error) {
	return rt.storage.GetTask(ctx, taskID)
}

// Events 返回 run 的事件流（FR-009）。
func (rt *Runtime) Events(ctx context.Context, runID string) ([]RuntimeEvent, error) {
	return rt.storage.Events(ctx, runID)
}

// AddArtifact 记录 run 的结构化产出。
func (rt *Runtime) AddArtifact(ctx context.Context, runID, name string, typ ArtifactType, content string) (*Artifact, error) {
	a := &Artifact{ID: newID("art"), TaskRunID: runID, Name: name, Type: typ, Content: content, CreatedAt: time.Now()}
	if err := rt.storage.AddArtifact(ctx, a); err != nil {
		return nil, err
	}
	_ = rt.addEvent(ctx, &TaskRun{ID: runID}, EventArtifactCreated, map[string]any{"name": name})
	return a, nil
}

// AddEvidence 记录 Artifact 的来源证据。
func (rt *Runtime) AddEvidence(ctx context.Context, artifactID, source, quote string) error {
	return rt.storage.AddEvidence(ctx, &Evidence{ArtifactID: artifactID, Source: source, Quote: quote})
}

// Close 关闭底层存储。
func (rt *Runtime) Close() error { return rt.storage.Close() }

// ---- 内部辅助 ----

func (rt *Runtime) getPattern(name string) (Pattern, bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	p, ok := rt.patterns[name]
	return p, ok
}

func (rt *Runtime) transition(ctx context.Context, run *TaskRun, to RunState) error {
	if !CanTransition(run.State, to) {
		return &StateError{From: run.State, To: to, Op: "transition"}
	}
	from := run.State
	run.State = to
	run.UpdatedAt = time.Now()
	_ = rt.addEvent(ctx, run, EventStateChanged, map[string]any{"from": from, "to": to})
	return rt.storage.UpdateRun(ctx, run)
}

func (rt *Runtime) fail(ctx context.Context, run *TaskRun, err error) error {
	run.Error = err.Error()
	if cerr := rt.transition(ctx, run, RunStateFailed); cerr != nil {
		return cerr
	}
	_ = rt.addEvent(ctx, run, EventTaskRunFailed, map[string]any{"error": err.Error()})
	return rt.storage.UpdateRun(ctx, run)
}

func (rt *Runtime) cancelRun(ctx context.Context, run *TaskRun, reason string) error {
	_ = rt.addEvent(ctx, run, EventTaskRunCancelled, map[string]any{"reason": reason})
	return rt.transition(ctx, run, RunStateCancelled)
}

func (rt *Runtime) runEvaluators(ctx context.Context, run *TaskRun) error {
	rt.mu.Lock()
	names := make([]string, 0, len(rt.evaluators))
	for n := range rt.evaluators {
		names = append(names, n)
	}
	rt.mu.Unlock()
	var allPassed = true
	for _, n := range names {
		rt.mu.Lock()
		e := rt.evaluators[n]
		rt.mu.Unlock()
		res, err := e.Evaluate(ctx, run, rt.storage)
		if err != nil {
			return err
		}
		if !res.Passed {
			allPassed = false
		}
		_ = rt.addEvent(ctx, run, EventEvaluatorResult, map[string]any{
			"evaluator": n, "passed": res.Passed, "score": res.Score, "details": res.Details,
		})
	}
	_ = allPassed
	return nil
}

func (rt *Runtime) addEvent(ctx context.Context, run *TaskRun, kind EventKind, data map[string]any) error {
	ev := &RuntimeEvent{ID: newID("evt"), TaskRunID: run.ID, Kind: kind, Data: data, Timestamp: time.Now()}
	return rt.storage.AddEvent(ctx, ev)
}

func mergeUsage(dst *agentmodel.Usage, u Usage) {
	dst.PromptTokens += u.PromptTokens
	dst.CompletionTokens += u.CompletionTokens
	dst.TotalTokens += u.TotalTokens
}

// ---- ID 生成（时间戳 + 计数器，避免依赖外部） ----

var idMu sync.Mutex
var idCounter int

func newID(prefix string) string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), idCounter)
}

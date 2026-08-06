package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Runtime 是 Task Runtime 入口：提交 Task、创建 TaskRun、执行、暂停/恢复、
// 取消、人工输入、事件与 Artifact 持久化（FR-004）。
//
// 并发安全：每个 runID 有独立的 per-run 锁（runLocks），
// Run/SubmitHumanInput 全程持锁防止重复执行；
// Cancel 通过 runCancels 触发执行中 Run 的上下文取消（不阻塞），
// 并从 storage 将状态迁移到 cancelled。
type Runtime struct {
	storage    Storage
	patterns   map[string]Pattern
	evaluators map[string]Evaluator
	budget     Budget

	mu         sync.Mutex
	runLocks   map[string]*sync.Mutex
	runCancels map[string]context.CancelFunc
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
		runLocks:   make(map[string]*sync.Mutex),
		runCancels: make(map[string]context.CancelFunc),
	}, nil
}

// lockRun 获取 runID 对应的 per-run 锁。锁不释放删除（避免 ABA），
// run 数量有限时可接受；长生命周期场景由调用方控制。
func (rt *Runtime) lockRun(id string) *sync.Mutex {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	l, ok := rt.runLocks[id]
	if !ok {
		l = &sync.Mutex{}
		rt.runLocks[id] = l
	}
	return l
}

// registerCancel 注册执行中 Run 的取消函数（Cancel API 使用）。
func (rt *Runtime) registerCancel(id string, fn context.CancelFunc) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.runCancels[id] = fn
}

func (rt *Runtime) unregisterCancel(id string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	delete(rt.runCancels, id)
}

// cancelExecuting 触发执行中 Run 的上下文取消（如无则返回 false）。
func (rt *Runtime) cancelExecuting(id string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	fn, ok := rt.runCancels[id]
	if ok && fn != nil {
		fn()
		return true
	}
	return false
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
		TaskInput: task.Input,
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
// 并发安全：per-run 锁保证同一 run 不会被重复执行。
func (rt *Runtime) Run(ctx context.Context, runID string) (*TaskRun, error) {
	l := rt.lockRun(runID)
	l.Lock()
	defer l.Unlock()
	return rt.runLocked(ctx, runID)
}

func (rt *Runtime) runLocked(ctx context.Context, runID string) (*TaskRun, error) {
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

	// 注册取消函数：必须在状态转移之前注册，并在转移前重读 storage——
	// 若 Cancel 恰在注册窗口内写入 cancelled，重读能发现并放弃执行，
	// 避免后续 transition/checkpoint 覆盖用户取消。
	runCtx, cancelFn := context.WithCancel(ctx)
	rt.registerCancel(runID, cancelFn)
	defer func() {
		cancelFn()
		rt.unregisterCancel(runID)
	}()

	cur, err := rt.storage.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if cur.State == RunStateCancelled {
		cancelFn()
		return cur, nil
	}

	if run.State != RunStateRunning {
		if cur, err := rt.transitionOrReturn(ctx, run, RunStateRunning); cur != nil {
			return cur, err
		}
		_ = rt.addEvent(ctx, run, EventTaskRunStarted, nil)
	}

	pattern, ok := rt.getPattern(run.Pattern)
	if !ok {
		_ = rt.fail(runCtx, run, fmt.Errorf("Pattern 未注册"))
		return rt.storage.GetRun(ctx, runID)
	}

	for {
		if err := runCtx.Err(); err != nil {
			_ = rt.cancelRun(runCtx, run, "context cancelled")
			return rt.storage.GetRun(ctx, runID)
		}
		if budgetExceeded(rt.budget, run.Iterations, run.ToolCalls) {
			_ = rt.addEvent(ctx, run, EventBudgetExceeded, map[string]any{
				"iterations": run.Iterations, "tool_calls": run.ToolCalls,
			})
			_ = rt.fail(ctx, run, fmt.Errorf("预算耗尽（iterations=%d tool_calls=%d）", run.Iterations, run.ToolCalls))
			return rt.storage.GetRun(ctx, runID)
		}

		step, err := pattern.Execute(runCtx, run)
		if err != nil {
			if runCtx.Err() != nil {
				// 取消优先于失败：执行中被取消 → cancelled
				_ = rt.cancelRun(runCtx, run, "context cancelled")
			} else {
				_ = rt.fail(ctx, run, err)
			}
			return rt.storage.GetRun(ctx, runID)
		}
		run.Iterations += step.Iterations
		run.ToolCalls += step.ToolCalls
		mergeUsage(&run.Usage, step.Usage)
		run.UpdatedAt = time.Now()
		if step.Progress != "" {
			run.Progress = step.Progress
		}

		// 业务终止（如审批拒绝）：按失败终态收敛，不进入完成/Evaluator 路径（S3）。
		if step.Terminated {
			_ = rt.fail(ctx, run, fmt.Errorf("流程终止: %s", step.Output))
			return rt.storage.GetRun(ctx, runID)
		}

		// 原子落库本步骤产出的 Artifact 与 Evidence（S4：单事务，不留脏数据）
		if len(step.Artifacts) > 0 || len(step.Evidence) > 0 {
			arts := make([]Artifact, len(step.Artifacts))
			copy(arts, step.Artifacts)
			for i := range arts {
				arts[i].ID = newID("art")
				arts[i].TaskRunID = run.ID
				if arts[i].CreatedAt.IsZero() {
					arts[i].CreatedAt = time.Now()
				}
			}
			// 名称 → 真实 ID：本步骤产物优先，miss 时补查 run 已有 Artifacts（S2）
			byName := map[string]string{}
			for _, a := range arts {
				byName[a.Name] = a.ID
			}
			evs := make([]Evidence, 0, len(step.Evidence))
			for _, ev := range step.Evidence {
				id, ok := byName[ev.ArtifactID]
				if !ok {
					existing, err := rt.storage.Artifacts(ctx, run.ID)
					if err != nil {
						_ = rt.fail(ctx, run, err)
						return rt.storage.GetRun(ctx, runID)
					}
					for _, a := range existing {
						if a.Name == ev.ArtifactID {
							id, ok = a.ID, true
							break
						}
					}
				}
				if !ok {
					_ = rt.fail(ctx, run, fmt.Errorf("evidence 引用未知 artifact %q", ev.ArtifactID))
					return rt.storage.GetRun(ctx, runID)
				}
				ev.ArtifactID = id
				evs = append(evs, ev)
			}
			if err := rt.storage.AddArtifactsEvidence(ctx, arts, evs); err != nil {
				_ = rt.fail(ctx, run, err)
				return rt.storage.GetRun(ctx, runID)
			}
			for _, a := range arts {
				_ = rt.addEvent(ctx, run, EventArtifactCreated, map[string]any{"name": a.Name})
			}
		}

		if step.NeedHuman {
			if cur, err := rt.transitionOrReturn(ctx, run, RunStateWaiting); cur != nil {
				return cur, err
			}
			_ = rt.addEvent(ctx, run, EventHumanInputRequested, map[string]any{"prompt": step.HumanPrompt})
			// Checkpoint：仅在 waiting 态可写——取消后（cancelled）不覆盖
			if ok, err := rt.storage.UpdateRunIf(ctx, run, RunStateWaiting); err != nil {
				_ = rt.fail(ctx, run, err)
				return rt.storage.GetRun(ctx, runID)
			} else if !ok {
				cur, gerr := rt.storage.GetRun(ctx, runID)
				if gerr != nil {
					return nil, gerr
				}
				return cur, nil
			}
			_ = rt.addEvent(ctx, run, EventCheckpointSaved, nil)
			return rt.storage.GetRun(ctx, runID)
		}

		if step.Output != "" {
			summaryAppend(run, step.Output)
		}
		// Checkpoint：仅在 running 态可写——取消后（cancelled）不覆盖
		if ok, err := rt.storage.UpdateRunIf(ctx, run, RunStateRunning); err != nil {
			_ = rt.fail(ctx, run, err)
			return rt.storage.GetRun(ctx, runID)
		} else if !ok {
			cur, gerr := rt.storage.GetRun(ctx, runID)
			if gerr != nil {
				return nil, gerr
			}
			return cur, nil
		}
		_ = rt.addEvent(ctx, run, EventCheckpointSaved, nil)

		if step.Done {
			break
		}
	}

	// 完成 + Evaluator 验收（任一 Evaluator 未通过 → failed）
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
	passed, err := rt.runEvaluators(ctx, run)
	if err != nil {
		_ = rt.fail(ctx, run, err)
		return rt.storage.GetRun(ctx, runID)
	}
	if !passed {
		_ = rt.fail(ctx, run, fmt.Errorf("Evaluator 验收未通过"))
		return rt.storage.GetRun(ctx, runID)
	}
	// 先持久化 Result（含全部数据）再 CAS completed——
	// UpdateRunIf(running) 在取消（cancelled）后不覆盖，Result 仅在正常完成路径写入。
	if ok, err := rt.storage.UpdateRunIf(ctx, run, RunStateRunning); err != nil {
		_ = rt.fail(ctx, run, err)
		return rt.storage.GetRun(ctx, runID)
	} else if !ok {
		cur, gerr := rt.storage.GetRun(ctx, runID)
		if gerr != nil {
			return nil, gerr
		}
		return cur, nil
	}
	if cur, err := rt.transitionOrReturn(ctx, run, RunStateCompleted); cur != nil {
		return cur, err
	}
	_ = rt.addEvent(ctx, run, EventTaskRunCompleted, map[string]any{"summary": run.Result.Summary})
	return rt.storage.GetRun(ctx, runID)
}

// SubmitHumanInput 向 waiting 的 TaskRun 提供人工输入（FR-004 HITL / S6 前置）。
// 并发安全：per-run 锁防止双提交重复执行。
func (rt *Runtime) SubmitHumanInput(ctx context.Context, runID, input string) (*TaskRun, error) {
	l := rt.lockRun(runID)
	l.Lock()
	defer l.Unlock()

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
	// 仅在 waiting 态可写——取消后（cancelled）不复活 run
	if ok, err := rt.storage.UpdateRunIf(ctx, run, RunStateWaiting); err != nil {
		return nil, err
	} else if !ok {
		cur, gerr := rt.storage.GetRun(ctx, runID)
		if gerr != nil {
			return nil, gerr
		}
		return cur, nil
	}
	_ = rt.addEvent(ctx, run, EventHumanInputReceived, map[string]any{"run_id": runID})
	if cur, err := rt.transitionOrReturn(ctx, run, RunStateRunning); cur != nil {
		return cur, err
	}
	// 已持 per-run 锁，直接执行（不重复加锁）
	return rt.runLocked(ctx, runID)
}

// Cancel 取消运行（终态幂等）。
// 对执行中的 Run：触发其上下文取消（pattern 中断），并把状态迁移到 cancelled。
// CAS 失败（状态已被并发迁移）时返回 storage 最新状态，取消不静默丢失。
func (rt *Runtime) Cancel(ctx context.Context, runID string) (*TaskRun, error) {
	run, err := rt.storage.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	switch run.State {
	case RunStateCompleted, RunStateFailed, RunStateCancelled:
		return run, nil
	}
	// 触发执行中 Run 的取消（不阻塞；Run 检测到后自行迁移终态）
	rt.cancelExecuting(runID)
	if err := rt.cancelRun(ctx, run, "user cancelled"); err != nil {
		// CAS 失败（并发迁移/非法转移）：返回最新状态而非静默成功
		cur, gerr := rt.storage.GetRun(context.WithoutCancel(ctx), runID)
		if gerr != nil {
			return nil, err
		}
		return cur, nil
	}
	// 成功分支同样用 WithoutCancel：调用方 ctx 已取消时仍能读到已写入的 cancelled 状态
	return rt.storage.GetRun(context.WithoutCancel(ctx), runID)
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
	// 原子 CAS：若 storage 状态已被并发修改（如外部 Cancel 写 cancelled），
	// 本迁移失败且不会覆盖；调用方据此收敛。
	sc := context.WithoutCancel(ctx)
	ok, err := rt.storage.CompareAndSetState(sc, run.ID, from, to)
	if err != nil {
		return err
	}
	if !ok {
		if cur, gerr := rt.storage.GetRun(sc, run.ID); gerr == nil {
			run.State = cur.State
			run.UpdatedAt = cur.UpdatedAt
		}
		return &StateError{From: from, To: to, Op: "transition (状态已被并发修改)"}
	}
	run.State = to
	run.UpdatedAt = time.Now()
	_ = rt.addEvent(sc, run, EventStateChanged, map[string]any{"from": from, "to": to})
	return nil
}

// transitionOrReturn 执行状态迁移。
// CAS 冲突（并发修改，如外部取消）→ 返回 storage 最新状态且不视为错误；
// 其它错误（storage 失败、非法转移）→ 透传。
func (rt *Runtime) transitionOrReturn(ctx context.Context, run *TaskRun, to RunState) (*TaskRun, error) {
	if err := rt.transition(ctx, run, to); err != nil {
		var se *StateError
		if errors.As(err, &se) {
			cur, gerr := rt.storage.GetRun(context.WithoutCancel(ctx), run.ID)
			if gerr != nil {
				return nil, err
			}
			return cur, nil
		}
		return nil, err
	}
	return nil, nil
}

func (rt *Runtime) fail(ctx context.Context, run *TaskRun, err error) error {
	run.Error = err.Error()
	// 取消路径的 storage 写必须用 WithoutCancel：
	// 已取消的 ctx 会让 SQLite ExecContext 立即失败，导致终态写不进去。
	sc := context.WithoutCancel(ctx)
	if cerr := rt.transition(sc, run, RunStateFailed); cerr != nil {
		return cerr
	}
	_ = rt.addEvent(sc, run, EventTaskRunFailed, map[string]any{"error": err.Error()})
	return rt.storage.UpdateRun(sc, run)
}

func (rt *Runtime) cancelRun(ctx context.Context, run *TaskRun, reason string) error {
	sc := context.WithoutCancel(ctx)
	// 先 CAS 迁移再写 cancelled 事件——CAS 失败（并发迁移）时不留污染事件。
	if err := rt.transition(sc, run, RunStateCancelled); err != nil {
		return err
	}
	_ = rt.addEvent(sc, run, EventTaskRunCancelled, map[string]any{"reason": reason})
	return nil
}

func (rt *Runtime) runEvaluators(ctx context.Context, run *TaskRun) (bool, error) {
	rt.mu.Lock()
	names := make([]string, 0, len(rt.evaluators))
	for n := range rt.evaluators {
		names = append(names, n)
	}
	rt.mu.Unlock()
	allPassed := true
	for _, n := range names {
		rt.mu.Lock()
		e := rt.evaluators[n]
		rt.mu.Unlock()
		res, err := e.Evaluate(ctx, run, rt.storage)
		if err != nil {
			return false, err
		}
		if !res.Passed {
			allPassed = false
		}
		_ = rt.addEvent(ctx, run, EventEvaluatorResult, map[string]any{
			"evaluator": n, "passed": res.Passed, "score": res.Score, "details": res.Details,
		})
	}
	return allPassed, nil
}

func (rt *Runtime) addEvent(ctx context.Context, run *TaskRun, kind EventKind, data map[string]any) error {
	ev := &RuntimeEvent{ID: newID("evt"), TaskRunID: run.ID, Kind: kind, Data: data, Timestamp: time.Now()}
	return rt.storage.AddEvent(ctx, ev)
}

// mergeUsage 将单步 Usage 累加到 run 的累计 Usage。
func mergeUsage(dst *agentmodel.Usage, u agentmodel.Usage) {
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

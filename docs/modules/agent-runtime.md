# agent-runtime — Task Runtime

`agent-runtime` 把一次 Agent 交互提升为**可管理、可验证、可恢复的 Task**：`TaskRun` 状态机、Pattern Host、预算、取消、事件审计、Artifact/Evidence、Evaluator 验收、HITL 人工介入，以及 Storage（内存/SQLite）持久化与 Checkpoint 恢复。

模块路径：`github.com/mantis-layer/mts/agent-runtime`（依赖 `agent-model`、`agent-core`）

## 设计原理

1. **Task 与 Agent 边界清晰**：`Task` 是任务定义（不包含执行计划），`TaskRun` 是运行实例；`Pattern` 负责"下一步做什么"，Runtime 负责"如何安全地做、如何记下来、如何验收"。
2. **状态机 + CAS 原子迁移**：`RunState` 有明确合法转移表（`CanTransition`）；所有状态迁移经 Storage 的 **CAS**（`CompareAndSetState`，SQLite 为 `UPDATE ... WHERE state=?`），数据写入用 **`UpdateRunIf`**（带状态条件）。因此并发 `Run`/`Cancel`/`SubmitHumanInput` 不会互相覆盖终态——取消不能被 checkpoint 反转。
3. **Pattern 只返回 `StepResult`**：Runtime 统一做预算统计、状态迁移、事件审计、Artifact/Evidence 原子落库（`AddArtifactsEvidence` 单事务，失败不留孤立数据）。ToolLoop/Research/Workflow 三个 Pattern 共享 Core（S5）。
4. **Checkpoint = 状态 + 事件**：每步执行后 `UpdateRunIf` + `AddEvent`；`Progress` 字段由 Pattern 自定义，恢复时从同一进度继续（幂等，不重跑已执行步骤）。
5. **取消是信号不是状态**：`Cancel` API 触发执行中 Run 的上下文取消（`runCancels` 注册表），并用 CAS 把状态收敛到 `cancelled`；取消路径写 storage 用 `context.WithoutCancel`（否则 SQLite `ExecContext` 会因已取消 ctx 立即失败）。
6. **SQLite 单写者**：单连接 + `busy_timeout(10000)`，避免并发写触发 `database is locked` 导致终态静默丢失。
7. **Storage 契约不泄漏实现**：接口无 SQLite 概念；数据保留/删除/TTL 明确延后（v0.1 只写不删）。

## 状态机

```text
created ──→ running ──→ completed
   │           │  └──→ failed
   │           │  └──→ cancelled
   └──→ cancelled        ↑
running ──→ waiting（HITL）──→ running
waiting ──→ cancelled / failed
```

终态（`completed`/`failed`/`cancelled`）不可再转移。非法转移返回 `StateError`；并发 CAS 冲突返回最新状态。

## 核心类型

### `Task` / `TaskRun` / `RunState`

```go
type Task struct {
    ID, Name, Pattern string // Pattern 选择 Pattern Host 中的执行者
    Input string
    CreatedAt time.Time
}

type RunState string
const ( RunStateCreated RunState = "created"; RunStateRunning; RunStateWaiting; RunStateCompleted; RunStateFailed; RunStateCancelled )

func CanTransition(from, to RunState) bool

type TaskRun struct {
    ID, TaskID, Pattern string
    State   RunState
    Iterations, ToolCalls int
    Usage   agentmodel.Usage
    Summary string     // 逐步追加的输出
    Progress string    // Pattern 自定义进度（持久化）
    TaskInput string   // 任务输入
    Input   string     // HITL 人工输入（一次性）
    Result  *TaskResult
    Error   string
    CreatedAt, UpdatedAt time.Time
}

type TaskResult struct {
    Summary string; Usage agentmodel.Usage
    ToolCalls, Iterations int
    Artifacts []Artifact
}
```

### `Runtime`

```go
func NewRuntime(storage Storage, budget Budget) (*Runtime, error)

func (rt *Runtime) RegisterPattern(p Pattern) error
func (rt *Runtime) RegisterEvaluator(e Evaluator) error

func (rt *Runtime) SubmitTask(ctx context.Context, task *Task) (*TaskRun, error)
func (rt *Runtime) Run(ctx context.Context, runID string) (*TaskRun, error)          // 阻塞到终态或 waiting（幂等）
func (rt *Runtime) Cancel(ctx context.Context, runID string) (*TaskRun, error)       // 取消（终态幂等）
func (rt *Runtime) SubmitHumanInput(ctx context.Context, runID, input string) (*TaskRun, error) // HITL
func (rt *Runtime) GetRun(ctx context.Context, runID string) (*TaskRun, error)
func (rt *Runtime) GetTask(ctx context.Context, taskID string) (*Task, error)
func (rt *Runtime) Events(ctx context.Context, runID string) ([]RuntimeEvent, error) // 审计事件
func (rt *Runtime) AddArtifact(ctx context.Context, runID, name string, typ ArtifactType, content string) (*Artifact, error)
func (rt *Runtime) AddEvidence(ctx context.Context, artifactID, source, quote string) error
func (rt *Runtime) Close() error
```

### `Pattern` 与 `StepResult`

```go
type Pattern interface {
    Name() string
    Execute(ctx context.Context, run *TaskRun) (*StepResult, error)
}

type StepResult struct {
    Done      bool   // 完成
    NeedHuman bool   // 进入 waiting 等待人工
    Terminated bool  // 业务终止（如审批拒绝 → failed）
    HumanPrompt string
    Output    string
    Progress  string // 进度（持久化，幂等恢复）
    Artifacts []Artifact
    Evidence  []Evidence
    Iterations, ToolCalls int
    Usage     Usage
}

// 内置 Pattern
func NewToolLoopPattern(agent *agentcore.Agent) *ToolLoopPattern     // "tool_loop"：一次 Agent 循环
func NewResearchPattern(agent *agentcore.Agent) *ResearchPattern     // "research"：研究→报告 Artifact+Evidence
func NewWorkflowPattern(steps []WorkflowStep) *WorkflowPattern       // "workflow"：步骤+审批+跳过
```

### `Storage` 接口与实现

```go
type Storage interface {
    SaveTask / GetTask
    CreateRun / UpdateRun / GetRun
    UpdateRunIf(ctx, run *TaskRun, from RunState) (bool, error) // CAS 数据写（取消后不覆盖）
    AddEvent / Events
    AddArtifact / Artifacts / AddEvidence / Evidence
    AddArtifactsEvidence(ctx, arts []Artifact, evs []Evidence) error // 单事务
    CompareAndSetState(ctx, runID string, from, to RunState) (bool, error) // CAS 状态迁移
    Close() error
}
// 实现：NewMemoryStorage()；NewSQLiteStorage(path string)（含旧库 ALTER 迁移）
```

### `Artifact` / `Evidence` / `Evaluator`

```go
type ArtifactType string // text | json | table
type Artifact struct { ID, TaskRunID, Name string; Type ArtifactType; Content string; CreatedAt time.Time }
type Evidence struct { ArtifactID, Source, Quote string }

type Evaluator interface {
    Name() string
    Evaluate(ctx context.Context, run *TaskRun, store Storage) (*EvaluationResult, error)
}
type EvaluationResult struct { Passed bool; Score float64; Details map[string]any }

// 内置 Evaluator：结构体字面量构造后经 RegisterEvaluator 注册
&SchemaEvaluator{ArtifactName: "report"}                 // 验证 Artifact 是合法 JSON
&EvidenceCoverageEvaluator{Required: 1}                  // 证据覆盖率（evidence / required）
```

### 事件（审计）

`EventKind`：`taskrun.created/started/state_changed/completed/failed/cancelled`、`budget_exceeded`、`human_input_requested/received`、`checkpoint_saved`、`artifact.created`、`evaluator_result`。

## 使用示例

```go
rt, _ := agentruntime.NewRuntime(agentruntime.NewSQLiteStorage("tasks.db"), agentruntime.Budget{MaxToolCalls: 10})
_ = rt.RegisterPattern(agentruntime.NewToolLoopPattern(agent))
_ = rt.RegisterEvaluator(agentruntime.NewEvidenceCoverageEvaluator(1))

run, _ := rt.SubmitTask(ctx, &agentruntime.Task{ID: "t1", Pattern: "tool_loop", Input: "分析 data.json"})
final, _ := rt.Run(ctx, run.ID)
// final.State == completed；final.Result.Summary；rt.Events(ctx, run.ID) 审计
```

### Workflow（人工审批 + 恢复）

```go
steps := []agentruntime.WorkflowStep{
    {Name: "准备", Action: prep},
    {Name: "审批", Human: true, Prompt: "是否发布？"},
    {Name: "发布", Action: deploy},
}
_ = rt.RegisterPattern(agentruntime.NewWorkflowPattern(steps))

waiting, _ := rt.Run(ctx, run.ID)          // 停在 waiting
approved, _ := rt.SubmitHumanInput(ctx, run.ID, "批准") // 或 "拒绝" → failed
// 进程重启：NewRuntime(同 SQLiteStorage) 后 GetRun → Progress 保留，从同一进度继续
```

## 并发模型（为什么这样设计）

| 场景 | 机制 |
|---|---|
| 同一 run 被并发 Run | per-run 锁（`runLocks`）串行化，防重复执行 |
| 执行中调用 Cancel | `runCancels` 触发 context 取消 → Pattern 中断 → CAS 收敛 cancelled |
| Cancel 与 checkpoint 竞争 | `UpdateRunIf(running)` 在 cancelled 后返回 `false`，不覆盖取消 |
| 双提交 HITL | per-run 锁 + `SubmitHumanInput` 只在 `waiting` 时生效 |
| SQLite 并发写 | 单连接 + `busy_timeout`（`database is locked` 曾静默吞掉终态） |
| 取消路径写终态 | `context.WithoutCancel`（已取消 ctx 会让 ExecContext 立即失败） |

## 边界

- 不包含具体模型与业务工具；`cli`/`examples` v0.1 尚不依赖 runtime（后续版本接入）。
- 数据保留/删除/TTL 延后；不做多租户/权限（由上层提供）。

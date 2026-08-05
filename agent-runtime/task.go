// Package agentruntime 提供通用 Task Runtime（FR-004）：
// Task/TaskRun、Run 状态机、预算、取消、事件订阅、Artifact/Evidence、
// Evaluator、基础 Checkpoint 持久化恢复与 Human-in-the-loop。
//
// Pattern 只决定下一步行为（FR-005），不负责持久化、工具执行与预算统计。
package agentruntime

import (
	"fmt"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// RunState 是 TaskRun 的生命周期状态。
type RunState string

const (
	RunStateCreated   RunState = "created"
	RunStateRunning   RunState = "running"
	RunStateWaiting   RunState = "waiting" // HITL：等待人工输入
	RunStateCompleted RunState = "completed"
	RunStateFailed    RunState = "failed"
	RunStateCancelled RunState = "cancelled"
)

// allowedTransitions 是合法状态转移表；终态（completed/failed/cancelled）不可再转移。
var allowedTransitions = map[RunState]map[RunState]bool{
	RunStateCreated: {RunStateRunning: true, RunStateCancelled: true},
	RunStateRunning: {RunStateWaiting: true, RunStateCompleted: true, RunStateFailed: true, RunStateCancelled: true},
	RunStateWaiting: {RunStateRunning: true, RunStateCancelled: true},
}

// CanTransition 判断 from → to 是否合法（FR-004 状态机）。
func CanTransition(from, to RunState) bool {
	dst, ok := allowedTransitions[from]
	return ok && dst[to]
}

// StateError 是非法状态操作的结构化错误。
type StateError struct {
	From RunState
	To   RunState
	Op   string
}

func (e *StateError) Error() string {
	return fmt.Sprintf("agentruntime: %s: 非法状态转移 %s → %s", e.Op, e.From, e.To)
}

// Task 是用户提交的任务定义。
type Task struct {
	ID        string
	Name      string
	Pattern   string // 使用的 Pattern 名
	Input     string // 用户任务输入
	CreatedAt time.Time
}

// TaskRun 是 Task 的一次执行实例（可暂停/恢复/查询）。
type TaskRun struct {
	ID         string
	TaskID     string
	Pattern    string // 使用的 Pattern 名（SubmitTask 时从 Task 复制）
	State      RunState
	Iterations int
	ToolCalls  int
	Usage      agentmodel.Usage
	Summary    string // 运行期逐步追加的输出
	Input      string // HITL：最近一次人工输入
	Result     *TaskResult
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// SummaryAppend 追加输出到 run 的 Summary。
func (r *TaskRun) SummaryAppend(s string) {
	if r.Summary != "" {
		r.Summary += "\n"
	}
	r.Summary += s
}

// TaskResult 是一次 TaskRun 的产出。
type TaskResult struct {
	Summary    string
	Usage      agentmodel.Usage
	ToolCalls  int
	Iterations int
	Artifacts  []Artifact
}

// Clone 深拷贝 run（避免共享可变字段）。
func (r *TaskRun) Clone() *TaskRun {
	if r == nil {
		return nil
	}
	out := *r
	if r.Result != nil {
		res := *r.Result
		res.Artifacts = append([]Artifact(nil), r.Result.Artifacts...)
		out.Result = &res
	}
	return &out
}

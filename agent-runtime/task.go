package agentruntime

import (
	"fmt"

	contract "github.com/mantis-layer/mts/agent-contract"
)

// RunState / Task / TaskRun / TaskResult 上移至 agent-contract；
// runtime 通过 type alias 保持 API 兼容。
type (
	RunState   = contract.RunState
	Task       = contract.Task
	TaskRun    = contract.TaskRun
	TaskResult = contract.TaskResult
)

// Re-export constants。
const (
	RunStateCreated   = contract.RunStateCreated
	RunStateRunning   = contract.RunStateRunning
	RunStateWaiting   = contract.RunStateWaiting
	RunStateCompleted = contract.RunStateCompleted
	RunStateFailed    = contract.RunStateFailed
	RunStateCancelled = contract.RunStateCancelled
)

// ---- 状态机（留在 runtime） ----

// allowedTransitions 是合法状态转移表；终态（completed/failed/cancelled）不可再转移。
var allowedTransitions = map[RunState]map[RunState]bool{
	RunStateCreated: {RunStateRunning: true, RunStateCancelled: true},
	RunStateRunning: {RunStateWaiting: true, RunStateCompleted: true, RunStateFailed: true, RunStateCancelled: true},
	RunStateWaiting: {RunStateRunning: true, RunStateCancelled: true, RunStateFailed: true},
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

// ---- 行为方法（内化为 runtime 内部函数） ----

// cloneRun 深拷贝 run（避免共享可变字段）；原 TaskRun.Clone 方法。
func cloneRun(r *TaskRun) *TaskRun {
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

// summaryAppend 追加输出到 run 的 Summary；原 TaskRun.SummaryAppend 方法。
func summaryAppend(r *TaskRun, s string) {
	if r.Summary != "" {
		r.Summary += "\n"
	}
	r.Summary += s
}

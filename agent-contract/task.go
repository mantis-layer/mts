package agentcontract

import (
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
	Progress   string // Pattern 自定义进度（持久化，支持幂等恢复）
	TaskInput  string // 任务输入（SubmitTask 时从 Task 复制；Pattern 步骤输入）
	Input      string // HITL：最近一次人工输入
	Result     *TaskResult
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TaskResult 是一次 TaskRun 的产出。
type TaskResult struct {
	Summary    string
	Usage      agentmodel.Usage
	ToolCalls  int
	Iterations int
	Artifacts  []Artifact
}

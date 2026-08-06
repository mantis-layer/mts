package agentcontract

import agentmodel "github.com/mantis-layer/mts/agent-model"

// StepResult 是一次 Pattern 执行的结果。
type StepResult struct {
	Done      bool // 任务完成
	NeedHuman bool // 需要人工输入
	// Terminated 表示流程被业务终止（如审批拒绝）：Runtime 按失败终态收敛，
	// 不进入完成/Evaluator 路径。
	Terminated  bool
	HumanPrompt string // 人工输入提示
	Output      string // 步骤输出（追加到 summary）
	// Progress 是 Pattern 自定义进度（如 Workflow 的步骤序号），由 Runtime 持久化。
	Progress string
	// Artifacts 是本步骤产出的结构化产出，Runtime 落库并绑定 run ID。
	Artifacts []Artifact
	// Evidence 是本步骤产出的来源证据（ArtifactID 引用，须与 Artifacts 对应）。
	Evidence []Evidence
	// Iterations 本步骤消耗的模型轮次；ToolCalls 工具调用次数；Usage token 消耗。
	Iterations int
	ToolCalls  int
	Usage      agentmodel.Usage
}

package agentruntime

import (
	"context"

	contract "github.com/mantis-layer/mts/agent-contract"
	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// StepResult 上移至 agent-contract；runtime 通过 type alias 保持 API 兼容。
type StepResult = contract.StepResult

// Pattern 决定 TaskRun 的下一步行为（FR-005）。
// Pattern 不负责持久化、工具执行与预算统计——这些由 Runtime 处理。
type Pattern interface {
	// Name 返回 Pattern 唯一标识。
	Name() string
	// Execute 执行一步，返回步骤结果。
	Execute(ctx context.Context, run *TaskRun) (*StepResult, error)
}

// ToolLoopPattern 包装 agent-core Agent 执行一次 Tool Loop（M4 基础 Pattern）。
type ToolLoopPattern struct {
	agent *agentcore.Agent
}

// NewToolLoopPattern 构造 ToolLoopPattern。
func NewToolLoopPattern(agent *agentcore.Agent) *ToolLoopPattern {
	return &ToolLoopPattern{agent: agent}
}

// Name 返回 Pattern 名。
func (p *ToolLoopPattern) Name() string { return "tool_loop" }

// Execute 把任务输入作为用户消息运行一次 Agent。
func (p *ToolLoopPattern) Execute(ctx context.Context, run *TaskRun) (*StepResult, error) {
	input := run.TaskInput
	if input == "" {
		input = "继续执行"
	}
	res, err := p.agent.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	return &StepResult{
		Done:       true,
		Output:     res.FinalMessage.Content,
		Iterations: res.Iterations,
		ToolCalls:  res.ToolCalls,
		Usage: agentmodel.Usage{
			PromptTokens:     res.Usage.PromptTokens,
			CompletionTokens: res.Usage.CompletionTokens,
			TotalTokens:      res.Usage.TotalTokens,
		},
	}, nil
}

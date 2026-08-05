package agentruntime

import (
	"context"

	agentcore "github.com/mantis-layer/mts/agent-core"
)

// Pattern 决定 TaskRun 的下一步行为（FR-005）。
// Pattern 不负责持久化、工具执行与预算统计——这些由 Runtime 处理。
type Pattern interface {
	// Name 返回 Pattern 唯一标识。
	Name() string
	// Execute 执行一步，返回步骤结果。
	Execute(ctx context.Context, run *TaskRun) (*StepResult, error)
}

// StepResult 是一次 Pattern 执行的结果。
type StepResult struct {
	Done        bool   // 任务完成
	NeedHuman   bool   // 需要人工输入
	HumanPrompt string // 人工输入提示
	Output      string // 步骤输出（追加到 summary）
	Iterations  int    // 本步骤消耗的模型轮次
	ToolCalls   int    // 本步骤消耗的工具调用次数
	Usage       Usage  // 本步骤的 token 消耗
}

// Usage 简化 token 计数（避免直接暴露 agent-model 结构）。
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
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

// Execute 把 run.Input 作为用户消息运行一次 Agent。
func (p *ToolLoopPattern) Execute(ctx context.Context, run *TaskRun) (*StepResult, error) {
	input := run.Input
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
		Usage: Usage{
			PromptTokens:     res.Usage.PromptTokens,
			CompletionTokens: res.Usage.CompletionTokens,
			TotalTokens:      res.Usage.TotalTokens,
		},
	}, nil
}

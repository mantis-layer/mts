package agentruntime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// WorkflowStep 是 Workflow 的一个步骤。
// 步骤顺序由 WorkflowPattern 维护；进度经 TaskRun.Progress 持久化（幂等恢复）。
type WorkflowStep struct {
	// Name 步骤名（审计与输出展示）。
	Name string
	// Action 执行动作；返回输出文本。nil 表示无动作（占位/分组）。
	Action func(ctx context.Context, input string) (string, error)
	// Human 表示人工审批节点（WAITING_HUMAN）：执行到此处等待人工输入。
	// 输入为拒绝词（拒绝/reject/no/不同意）时流程终止；否则视为批准继续。
	Human  bool
	Prompt string // 审批提示
	// SkipRule 是 Rule Evaluator：返回 true 时跳过本步骤（不执行 Action、不等待审批）。
	SkipRule func(ctx context.Context, run *TaskRun) bool
	// Artifacts / Evidence 是本步骤的结构化产出（执行后由 Runtime 原子落库）。
	Artifacts []Artifact
	Evidence  []Evidence
}

// WorkflowPattern 实现多步骤工作流（FR-005 Pattern），
// 含 Rule Evaluator、WAITING_HUMAN 审批、进度持久化与幂等恢复。
// Human Approval 不侵入 Agent Core——本 Pattern 仅依赖 runtime 层。
type WorkflowPattern struct {
	steps []WorkflowStep
}

// NewWorkflowPattern 构造 WorkflowPattern。
func NewWorkflowPattern(steps []WorkflowStep) *WorkflowPattern {
	return &WorkflowPattern{steps: steps}
}

// Name 返回 Pattern 名。
func (p *WorkflowPattern) Name() string { return "workflow" }

// Execute 执行当前进度对应的一步；返回 Done/NeedHuman/Progress。
// 进度（步骤序号）编码在 run.Progress，恢复时从同一序号继续。
func (p *WorkflowPattern) Execute(ctx context.Context, run *TaskRun) (*StepResult, error) {
	idx := workflowIndex(run.Progress)
	for idx < len(p.steps) {
		step := p.steps[idx]

		// Rule Evaluator：跳过条件
		if step.SkipRule != nil && step.SkipRule(ctx, run) {
			idx++
			return &StepResult{
				Output:   fmt.Sprintf("[%s] 跳过（规则命中）", step.Name),
				Progress: wfProgress(idx),
			}, nil
		}

		// WAITING_HUMAN 审批节点
		if step.Human {
			if run.Input == "" {
				return &StepResult{
					NeedHuman:   true,
					HumanPrompt: step.Prompt,
					Progress:    wfProgress(idx),
				}, nil
			}
			// 一次性输入：消费后清空，避免残留导致后续审批节点被静默放行（B1）。
			input := run.Input
			run.Input = ""
			if isReject(input) {
				idx = len(p.steps)
				return &StepResult{
					Terminated: true,
					Output:     fmt.Sprintf("[%s] 审批未通过，流程终止", step.Name),
					Progress:   wfProgress(idx),
				}, nil
			}
			idx++
			return &StepResult{
				Output:   fmt.Sprintf("[%s] 审批通过，继续", step.Name),
				Progress: wfProgress(idx),
			}, nil
		}

		// 常规动作步骤
		if step.Action == nil {
			idx++
			continue
		}
		out, err := step.Action(ctx, run.TaskInput)
		if err != nil {
			return nil, err
		}
		idx++
		return &StepResult{
			Output:    fmt.Sprintf("[%s] %s", step.Name, out),
			Progress:  wfProgress(idx),
			Artifacts: step.Artifacts,
			Evidence:  step.Evidence,
		}, nil
	}
	return &StepResult{Done: true}, nil
}

// workflowIndex 解析持久化进度（步骤序号）；非法/空视为 0。
func workflowIndex(progress string) int {
	if progress == "" {
		return 0
	}
	i, err := strconv.Atoi(progress)
	if err != nil || i < 0 {
		return 0
	}
	return i
}

func wfProgress(n int) string { return strconv.Itoa(n) }

// isReject 判断人工输入是否为拒绝意见。
func isReject(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "拒绝", "reject", "no", "不同意", "n":
		return true
	}
	return false
}

package agentruntime

import (
	"context"
	"strings"

	agentcore "github.com/mantis-layer/mts/agent-core"
)

// ResearchPattern 执行多轮研究（复用 Tool Loop Agent，检索源由已注册 Tool 提供），
// 把模型最终输出作为报告 Artifact，并附 Evidence（引用来源）。
//
// Evidence 的 ArtifactID 使用 Artifact 的 Name（如 "report"），
// Runtime 落库时映射为真实 Artifact ID。
type ResearchPattern struct {
	agent        *agentcore.Agent
	artifactName string // 报告 Artifact 名
}

// NewResearchPattern 构造 ResearchPattern。
// agent 需已注册检索类 Tool（如 File Reader）。
func NewResearchPattern(agent *agentcore.Agent) *ResearchPattern {
	return &ResearchPattern{agent: agent, artifactName: "report"}
}

// Name 返回 Pattern 名。
func (p *ResearchPattern) Name() string { return "research" }

// Execute 运行一轮研究：调用 Agent 完成任务，产出报告 Artifact + Evidence。
func (p *ResearchPattern) Execute(ctx context.Context, run *TaskRun) (*StepResult, error) {
	input := run.Input
	if input == "" {
		input = "继续研究并输出结论"
	}
	res, err := p.agent.Run(ctx, input)
	if err != nil {
		return nil, err
	}
	content := res.FinalMessage.Content
	artifacts := []Artifact{{
		Name:    p.artifactName,
		Type:    ArtifactText,
		Content: content,
	}}
	evidence := []Evidence{{
		ArtifactID: p.artifactName,
		Source:     "research:" + p.artifactName,
		Quote:      clip(content, 500),
	}}
	return &StepResult{
		Done:       true,
		Output:     content,
		Artifacts:  artifacts,
		Evidence:   evidence,
		Iterations: res.Iterations,
		ToolCalls:  res.ToolCalls,
		Usage: Usage{
			PromptTokens:     res.Usage.PromptTokens,
			CompletionTokens: res.Usage.CompletionTokens,
			TotalTokens:      res.Usage.TotalTokens,
		},
	}, nil
}

// clip 截断超长文本（Evidence Quote 防超长）。
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "…"
}

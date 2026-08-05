package agentruntime

import (
	"context"
	"strings"
	"testing"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// TestResearchPattern R1：研究 → 报告 Artifact + Evidence → Evaluator 验收通过。
func TestResearchPattern(t *testing.T) {
	for _, s := range newTestStorage(t) {
		m := &mockModel{streams: [][]agentmodel.StreamEvent{
			{ // 研究：读取数据源（file_reader）
				{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "tc1", Name: "file_reader", Arguments: `{"path":"/tmp/x.json"}`}},
				{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 8, CompletionTokens: 4, TotalTokens: 12}},
				{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
			},
			{ // 报告
				{Kind: agentmodel.StreamEventDelta, Delta: "研究结论：数据完整"},
				{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8}},
				{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop},
			},
		}}
		reg := agentcore.NewRegistry()
		if err := reg.Register(fileReaderTool{}); err != nil {
			t.Fatal(err)
		}
		agent := agentcore.New(m, reg, agentcore.Options{})
		pat := NewResearchPattern(agent)

		rt := newTestRuntime(t, s, Budget{})
		if err := rt.RegisterPattern(pat); err != nil {
			t.Fatal(err)
		}
		// 报告为文本 Artifact，用 EvidenceCoverage 验收（Schema 校验面向 JSON Artifact）
		rt.RegisterEvaluator(&EvidenceCoverageEvaluator{Required: 1})

		ctx := context.Background()
		run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "research", Input: "分析 /tmp/x.json 并输出报告"})
		final, err := rt.Run(ctx, run.ID)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if final.State != RunStateCompleted {
			t.Fatalf("终态 = %s，期望 completed", final.State)
		}
		// 报告 Artifact 已落库
		arts, err := rt.storage.Artifacts(ctx, run.ID)
		if err != nil || len(arts) != 1 || arts[0].Name != "report" {
			t.Fatalf("Artifacts = %v, %v", arts, err)
		}
		if !strings.Contains(arts[0].Content, "研究结论") {
			t.Fatalf("报告内容 = %q", arts[0].Content)
		}
		// Evidence 已关联到报告
		evs, err := rt.storage.Evidence(ctx, arts[0].ID)
		if err != nil || len(evs) != 1 {
			t.Fatalf("Evidence = %v, %v", evs, err)
		}
		// 结果含 usage 与 tool calls
		if final.Usage.TotalTokens != 20 {
			t.Fatalf("Usage = %+v", final.Usage)
		}
		if final.ToolCalls != 1 {
			t.Fatalf("ToolCalls = %d", final.ToolCalls)
		}
		// 输入透传：任务输入应作为用户消息传给模型（review should-fix 回归）
		m.mu.Lock()
		defer m.mu.Unlock()
		if len(m.requests) == 0 || len(m.requests[0].Messages) == 0 ||
			m.requests[0].Messages[0].Content != "分析 /tmp/x.json 并输出报告" {
			t.Fatalf("模型收到输入 = %+v", m.requests)
		}
	}
}

// fileReaderTool 测试用：读文件返回内容。
type fileReaderTool struct{}

func (fileReaderTool) Name() string        { return "file_reader" }
func (fileReaderTool) Description() string { return "读取文件" }
func (fileReaderTool) Parameters() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}}
}
func (fileReaderTool) Execute(_ context.Context, input map[string]any) (map[string]any, error) {
	return map[string]any{"content": "数据样本"}, nil
}

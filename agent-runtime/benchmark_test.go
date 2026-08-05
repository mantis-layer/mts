package agentruntime

import (
	"context"
	"testing"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// BenchmarkAgentLoop 基准：agent-core Agent 单次 Tool Loop（mock 流式模型）。
func BenchmarkAgentLoop(b *testing.B) {
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		{
			{Kind: agentmodel.StreamEventDelta, Delta: "完成"},
			{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
			{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop},
		},
	}}
	reg := agentcore.NewRegistry()
	reg.Register(echoTool{})
	agent := agentcore.New(m, reg, agentcore.Options{})
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := agent.Run(context.Background(), "hi"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkRuntimeSubmitRun 基准：Runtime 提交并执行一步完成的 Pattern。
func BenchmarkRuntimeSubmitRun(b *testing.B) {
	rt, err := NewRuntime(NewMemoryStorage(), Budget{})
	if err != nil {
		b.Fatal(err)
	}
	rt.RegisterPattern(&mockPattern{name: "mock", steps: []StepResult{{Done: true}}})
	ctx := context.Background()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		run, err := rt.SubmitTask(ctx, &Task{ID: "t", Pattern: "mock", Input: "x"})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := rt.Run(ctx, run.ID); err != nil {
			b.Fatal(err)
		}
	}
}

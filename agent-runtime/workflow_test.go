package agentruntime

import (
	"context"
	"strings"
	"sync"
	"testing"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

func wfSteps() []WorkflowStep {
	return []WorkflowStep{
		{Name: "准备", Action: func(_ context.Context, _ string) (string, error) { return "数据就绪", nil }},
		{Name: "审批", Human: true, Prompt: "是否批准发布？"},
		{Name: "发布", Action: func(_ context.Context, _ string) (string, error) { return "已发布", nil }},
	}
}

// TestWorkflowPattern R2：do → 人工审批（waiting）→ 批准继续完成。
func TestWorkflowPattern(t *testing.T) {
	for _, s := range newTestStorage(t) {
		rt := newTestRuntime(t, s, Budget{})
		if err := rt.RegisterPattern(NewWorkflowPattern(wfSteps())); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "发布 v1"})

		// 第一步执行，第二步等待审批
		waiting, err := rt.Run(ctx, run.ID)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if waiting.State != RunStateWaiting {
			t.Fatalf("状态 = %s，期望 waiting", waiting.State)
		}
		if !strings.Contains(waiting.Summary, "[准备]") {
			t.Fatalf("Summary = %q", waiting.Summary)
		}
		// 审批通过
		final, err := rt.SubmitHumanInput(ctx, run.ID, "批准")
		if err != nil {
			t.Fatalf("SubmitHumanInput: %v", err)
		}
		if final.State != RunStateCompleted {
			t.Fatalf("终态 = %s，期望 completed", final.State)
		}
		if !strings.Contains(final.Summary, "[审批] 审批通过") || !strings.Contains(final.Summary, "[发布] 已发布") {
			t.Fatalf("Summary = %q", final.Summary)
		}
	}
}

// TestWorkflowPattern_Reject R2+S3：审批拒绝 → 流程终止（failed，不进 Evaluator/完成路径）。
func TestWorkflowPattern_Reject(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(NewWorkflowPattern(wfSteps()))
	rt.RegisterEvaluator(&EvidenceCoverageEvaluator{Required: 1}) // 被拒流程不应触发 Evaluator
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "发布"})
	if _, err := rt.Run(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	final, err := rt.SubmitHumanInput(ctx, run.ID, "拒绝")
	if err != nil {
		t.Fatalf("SubmitHumanInput: %v", err)
	}
	// 拒绝 = 业务终止 → failed（非 completed，非 evaluator 判定的 failed）
	if final.State != RunStateFailed {
		t.Fatalf("拒绝后终态 = %s，期望 failed", final.State)
	}
	if !strings.Contains(final.Error, "审批未通过") {
		t.Fatalf("Error = %q", final.Error)
	}
	if strings.Contains(final.Summary, "[发布]") {
		t.Fatalf("拒绝后 Summary = %q（不应执行发布）", final.Summary)
	}
}

// TestWorkflowPattern_MultiApproval B1 回归：双审批节点，第二个必须重新进入 waiting。
func TestWorkflowPattern_MultiApproval(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "步骤A", Action: func(_ context.Context, _ string) (string, error) { return "A", nil }},
		{Name: "审批A", Human: true, Prompt: "批准 A？"},
		{Name: "步骤B", Action: func(_ context.Context, _ string) (string, error) { return "B", nil }},
		{Name: "审批B", Human: true, Prompt: "批准 B？"},
	}
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(NewWorkflowPattern(steps))
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "流程"})

	// 第一次等待：审批A
	w1, err := rt.Run(ctx, run.ID)
	if err != nil || w1.State != RunStateWaiting {
		t.Fatalf("第一次等待 = %v, %v", w1, err)
	}
	// 批准 A → 应停在审批B（而非静默放行）
	w2, err := rt.SubmitHumanInput(ctx, run.ID, "批准")
	if err != nil || w2.State != RunStateWaiting {
		t.Fatalf("批准 A 后 = %v, %v（期望再次 waiting 于审批B）", w2, err)
	}
	if !strings.Contains(w2.Summary, "[审批A] 审批通过") || !strings.Contains(w2.Summary, "[步骤B] B") {
		t.Fatalf("Summary = %q", w2.Summary)
	}
	// 批准 B → 完成
	final, err := rt.SubmitHumanInput(ctx, run.ID, "批准")
	if err != nil || final.State != RunStateCompleted {
		t.Fatalf("批准 B 后 = %v, %v", final, err)
	}
	if !strings.Contains(final.Summary, "[审批B] 审批通过") {
		t.Fatalf("Summary = %q", final.Summary)
	}
}

// TestWorkflowPattern_CrossStepEvidence S2 回归：步骤1 产 artifact，步骤2 引用它。
func TestWorkflowPattern_CrossStepEvidence(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "起草", Action: func(_ context.Context, _ string) (string, error) { return "草稿完成", nil },
			Artifacts: []Artifact{{Name: "draft", Type: ArtifactText, Content: "草稿"}}},
		{Name: "定稿", Action: func(_ context.Context, _ string) (string, error) { return "定稿完成", nil },
			Evidence: []Evidence{{ArtifactID: "draft", Source: "步骤1", Quote: "草稿引用"}}},
	}
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(NewWorkflowPattern(steps))
	rt.RegisterEvaluator(&EvidenceCoverageEvaluator{Required: 1})
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "写文档"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateCompleted {
		t.Fatalf("终态 = %s", final.State)
	}
	// 跨步骤 Evidence 已关联到真实 Artifact ID（EvidenceCoverage 通过即证明）
	arts, _ := rt.storage.Artifacts(ctx, run.ID)
	if len(arts) != 1 || arts[0].Name != "draft" {
		t.Fatalf("Artifacts = %v", arts)
	}
	evs, _ := rt.storage.Evidence(ctx, arts[0].ID)
	if len(evs) != 1 || evs[0].Source != "步骤1" {
		t.Fatalf("Evidence = %v", evs)
	}
}

// TestWorkflowPattern_RuleEvaluator R4：跳过条件生效。
func TestWorkflowPattern_RuleEvaluator(t *testing.T) {
	steps := []WorkflowStep{
		{Name: "检查", Action: func(_ context.Context, _ string) (string, error) { return "OK", nil }},
		{Name: "可选优化", Action: func(_ context.Context, _ string) (string, error) { return "已优化", nil },
			SkipRule: func(_ context.Context, run *TaskRun) bool { return strings.Contains(run.TaskInput, "快速") }},
	}
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})
	rt.RegisterPattern(NewWorkflowPattern(steps))
	ctx := context.Background()
	run, _ := rt.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "快速发布"})
	final, err := rt.Run(ctx, run.ID)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if final.State != RunStateCompleted {
		t.Fatalf("终态 = %s", final.State)
	}
	if !strings.Contains(final.Summary, "跳过") || strings.Contains(final.Summary, "已优化") {
		t.Fatalf("Summary = %q", final.Summary)
	}
}

// TestWorkflowPattern_Recovery R3：审批等待后重启，从同一进度继续（幂等）。
func TestWorkflowPattern_Recovery(t *testing.T) {
	db := t.TempDir() + "/wf.db"
	ctx := context.Background()

	var executed []string
	var mu sync.Mutex
	mkStep := func(name string) WorkflowStep {
		return WorkflowStep{Name: name, Action: func(_ context.Context, _ string) (string, error) {
			mu.Lock()
			executed = append(executed, name)
			mu.Unlock()
			return name + " 完成", nil
		}}
	}
	steps := []WorkflowStep{mkStep("准备"), {Name: "审批", Human: true, Prompt: "批准？"}, mkStep("发布")}

	rt1 := newTestRuntime(t, mustSQLite(t, db), Budget{})
	if err := rt1.RegisterPattern(NewWorkflowPattern(steps)); err != nil {
		t.Fatal(err)
	}
	run, _ := rt1.SubmitTask(ctx, &Task{ID: "t1", Pattern: "workflow", Input: "发布"})
	waiting, err := rt1.Run(ctx, run.ID)
	if err != nil || waiting.State != RunStateWaiting {
		t.Fatalf("第一次运行 = %v, %v", waiting, err)
	}
	if err := rt1.Close(); err != nil {
		t.Fatal(err)
	}

	// 新进程恢复：进度保留在 step 1（审批），不重跑"准备"
	rt2 := newTestRuntime(t, mustSQLite(t, db), Budget{})
	if err := rt2.RegisterPattern(NewWorkflowPattern(steps)); err != nil {
		t.Fatal(err)
	}
	recovered, err := rt2.GetRun(ctx, run.ID)
	if err != nil || recovered.State != RunStateWaiting || recovered.Progress != "1" {
		t.Fatalf("恢复 = %v, %v（Progress=%q）", recovered, err, recovered.Progress)
	}
	final, err := rt2.SubmitHumanInput(ctx, run.ID, "批准")
	if err != nil || final.State != RunStateCompleted {
		t.Fatalf("恢复后继续 = %v, %v", final, err)
	}
	mu.Lock()
	defer mu.Unlock()
	// "准备"只执行一次（幂等保护）
	if len(executed) != 2 || executed[0] != "准备" || executed[1] != "发布" {
		t.Fatalf("executed = %v（期望 [准备 发布]，不重复）", executed)
	}
}

// TestMultiPatternSharedRuntime R5/S5：三个 Pattern 同一 Runtime 各自运行。
func TestMultiPatternSharedRuntime(t *testing.T) {
	s := NewMemoryStorage()
	rt := newTestRuntime(t, s, Budget{})

	// Tool Loop
	reg := agentcore.NewRegistry()
	reg.Register(echoTool{})
	rt.RegisterPattern(NewToolLoopPattern(agentcore.New(&mockModel{streams: [][]agentmodel.StreamEvent{
		{{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "t", Name: "echo", Arguments: `{"msg":"hi"}`}}},
		{{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls}},
	}}, reg, agentcore.Options{})))

	// Research
	rt.RegisterPattern(NewResearchPattern(agentcore.New(&mockModel{streams: [][]agentmodel.StreamEvent{
		{{Kind: agentmodel.StreamEventDelta, Delta: "报告A"}},
		{{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop}},
	}}, agentcore.NewRegistry(), agentcore.Options{})))

	// Workflow
	rt.RegisterPattern(NewWorkflowPattern([]WorkflowStep{
		{Name: "步骤", Action: func(_ context.Context, _ string) (string, error) { return "完成", nil }},
	}))

	ctx := context.Background()
	for _, tc := range []struct {
		pattern string
		input   string
	}{
		{"tool_loop", "回显"},
		{"research", "研究"},
		{"workflow", "执行"},
	} {
		run, err := rt.SubmitTask(ctx, &Task{ID: "t-" + tc.pattern, Pattern: tc.pattern, Input: tc.input})
		if err != nil {
			t.Fatalf("SubmitTask(%s): %v", tc.pattern, err)
		}
		final, err := rt.Run(ctx, run.ID)
		if err != nil {
			t.Fatalf("Run(%s): %v", tc.pattern, err)
		}
		if final.State != RunStateCompleted {
			t.Fatalf("%s 终态 = %s", tc.pattern, final.State)
		}
	}
}

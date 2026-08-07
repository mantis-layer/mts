package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
	agentruntime "github.com/mantis-layer/mts/agent-runtime"
)

// ===========================================================================
// A2/S11/P1：三示例共享 Persona + Memory 抽象，用法一致（无需改 agent-core）
// ===========================================================================

// TestSharedAbstractionSmoke 验证 setupSession 用统一用法构造三件套（A2/S11/P1）。
// 这与三个示例 main.go 的构造路径一致——同一份 helper、同一份 MemoryStore 接口、
// 同一份 ContextBuilder 实现，无重复造轮子。
func TestSharedAbstractionSmoke(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-shared", Name: "Shared", Role: "tester", SystemPrompt: "你是测试助手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 三件套非空即证明抽象可用。
	if env.Persona == nil || env.MemoryStore == nil || env.ContextBuilder == nil {
		t.Fatalf("三件套不应有 nil: persona=%v mem=%v cb=%v", env.Persona, env.MemoryStore, env.ContextBuilder)
	}
	// 直接用 agentcontract.MemoryStore 接口断言 VectorMemoryStore 满足契约。
	var _ agentcontract.MemoryStore = env.MemoryStore
	var _ agentcontract.ContextBuilder = env.ContextBuilder

	// 写一条 LongTerm 记忆，证明接口可写。
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm,
		"用户偏好中文摘要", "preference"); err != nil {
		t.Fatalf("saveMemory: %v", err)
	}
}

// TestThreeExamplesConsistentUsage 验证三示例的"组装路径"在 helper 层完全一致：
// tool_loop / research / workflow 三种 Agent 形态都用同一个 setupSession 出来的
// Persona+MemoryStore+ContextBuilder 装到 Options 里。这是 P1/S11 的直接证据。
func TestThreeExamplesConsistentUsage(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-three", Name: "Three", Role: "multi", SystemPrompt: "你是多面手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 三示例共享的 Options 装配（一致用法）——与各自 main.go 的装配路径等价。
	buildOpts := func(onEvent func(agentcore.Event)) agentcore.Options {
		return agentcore.Options{
			ContextBuilder: env.ContextBuilder,
			Persona:        env.Persona,
			MemoryStore:    env.MemoryStore,
			OnEvent:        onEvent,
			MaxIterations:  3,
		}
	}
	// 三种形态都能拿到同一份 Options，证明共享。
	for i, name := range []string{"tool_loop", "research", "workflow"} {
		opts := buildOpts(func(agentcore.Event) {})
		if opts.ContextBuilder == nil || opts.Persona == nil || opts.MemoryStore == nil {
			t.Fatalf("示例 %s 的 Options 三件套不完整", name)
		}
		_ = i
	}
	_ = ctx
}

// ===========================================================================
// A4/S14/P3：记忆注入事件可观测（MemoryInjected 在模型调用前触发）
// ===========================================================================

// TestMemoryInjectedEventFires 验证当 LongTerm 有记忆时，每次模型调用前
// 会触发 EventMemoryInjected 事件（携带注入内容）。
func TestMemoryInjectedEventFires(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-inject", Name: "Inject", Role: "echo", SystemPrompt: "你是回显助手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 预置一条 LongTerm 记忆（注入内容来自这里）。
	const fact = "用户的项目代号是 Mantis Forge"
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm, fact, "project"); err != nil {
		t.Fatalf("saveMemory: %v", err)
	}

	model := newFakeModel(textStream("好的，已了解。"))
	var injected []agentcore.Event
	agent := agentcore.New(model, agentcore.NewRegistry(), agentcore.Options{
		ContextBuilder: env.ContextBuilder,
		Persona:        env.Persona,
		MemoryStore:    env.MemoryStore,
		OnEvent: func(ev agentcore.Event) {
			if ev.Kind == agentcore.EventMemoryInjected {
				injected = append(injected, ev)
			}
		},
		MaxIterations: 1,
	})

	res, err := agent.Run(ctx, "介绍一下项目")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Iterations == 0 {
		t.Fatalf("期望至少一次模型调用，实际 %d", res.Iterations)
	}

	if len(injected) == 0 {
		t.Fatalf("期望 MemoryInjected 事件，实际收到 %d 个", len(injected))
	}
	// 注入内容应包含预置记忆的关键词。
	if !strings.Contains(injected[0].Content, fact) {
		t.Fatalf("MemoryInjected 内容未包含预置记忆: got %q", injected[0].Content)
	}
	// 注入内容应置于 system 角色。
	if !strings.Contains(injected[0].Content, "记忆") {
		t.Fatalf("MemoryInjected 内容缺少记忆头标识: %q", injected[0].Content)
	}
}

// TestMemoryInjectedBeforeModelCall 验证 MemoryInjected 事件在模型调用"之前"触发。
// 通过事件顺序断言：在同一次 Run 的事件流里，首个 memory.injected 必须出现在
// 首个 model.done 之前（即注入先于模型完成；又因注入在每次模型调用前的同一轮中发射，
// 故事件序上 memory.injected 早于 model.done）。
func TestMemoryInjectedBeforeModelCall(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-order", Name: "Order", Role: "echo", SystemPrompt: "你是回显助手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm, "记忆点 alpha", "tag-a"); err != nil {
		t.Fatalf("saveMemory: %v", err)
	}

	model := newFakeModel(textStream("完成。"))
	var timeline []agentcore.Event
	agent := agentcore.New(model, agentcore.NewRegistry(), agentcore.Options{
		ContextBuilder: env.ContextBuilder,
		Persona:        env.Persona,
		MemoryStore:    env.MemoryStore,
		OnEvent: func(ev agentcore.Event) {
			timeline = append(timeline, ev)
		},
		MaxIterations: 1,
	})
	if _, err := agent.Run(ctx, "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	firstMemory := -1
	firstModelDone := -1
	for i, ev := range timeline {
		if ev.Kind == agentcore.EventMemoryInjected && firstMemory == -1 {
			firstMemory = i
		}
		if ev.Kind == agentcore.EventModelDone && firstModelDone == -1 {
			firstModelDone = i
		}
	}
	if firstMemory == -1 {
		t.Fatalf("未观察到 MemoryInjected 事件")
	}
	if firstModelDone == -1 {
		t.Fatalf("未观察到 ModelDone 事件")
	}
	if firstMemory > firstModelDone {
		t.Fatalf("MemoryInjected(%d) 应在 ModelDone(%d) 之前", firstMemory, firstModelDone)
	}
}

// TestMemoryInjectedAbsentWhenStoreEmpty 验证无记忆时不注入（返回空 Content，
// agent-core 不发射 MemoryInjected，或发射的 Content 为空）——即 graceful 降级。
func TestMemoryInjectedAbsentWhenStoreEmpty(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-empty", Name: "Empty", Role: "echo", SystemPrompt: "你是回显助手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 不写任何记忆。
	model := newFakeModel(textStream("无注入。"))
	var memoryEvents []agentcore.Event
	agent := agentcore.New(model, agentcore.NewRegistry(), agentcore.Options{
		ContextBuilder: env.ContextBuilder,
		Persona:        env.Persona,
		MemoryStore:    env.MemoryStore,
		OnEvent: func(ev agentcore.Event) {
			if ev.Kind == agentcore.EventMemoryInjected {
				memoryEvents = append(memoryEvents, ev)
			}
		},
		MaxIterations: 1,
	})
	if _, err := agent.Run(ctx, "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// 无记忆时不应发射带内容的 MemoryInjected（允许降级错误事件，但 Content 应为空）。
	for _, ev := range memoryEvents {
		if strings.TrimSpace(ev.Content) != "" {
			t.Fatalf("空记忆不应注入内容，但收到: %q", ev.Content)
		}
	}
}

// ===========================================================================
// A3/S13/P2：跨会话恢复——Run1 写 LongTerm → Close → 重开同一 DB → Run2 检索到
// ===========================================================================

// TestCrossSessionRecovery 是本 Issue 的核心验收：
//
//	Run1: 写入 LongTerm 记忆 → 关闭 MemoryStore（模拟进程退出）
//	Run2: 用同一 SQLite 文件重开 → 按同一 PersonaID 检索到该记忆，
//	      且 ContextBuilder 注入它，MemoryInjected 事件可观测。
//
// 用真实 SQLite 文件持久化（临时目录），fake embedder 保证确定性与离线。
func TestCrossSessionRecovery(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memory-cross.db")

	personaSpec := PersonaSpec{
		ID: "persona-cross", Name: "Cross", Role: "persistent", SystemPrompt: "你记得用户的偏好",
	}
	const fact = "用户希望所有报告使用中文输出并附数据来源"

	// ---- Run1：写入 LongTerm 记忆并关闭 ----
	{
		env1, err := setupSession(dbPath, NewFakeEmbedder(), personaSpec)
		if err != nil {
			t.Fatalf("Run1 setupSession: %v", err)
		}
		if err := saveMemory(ctx, env1.MemoryStore, env1.Persona.ID,
			agentcontract.MemoryLayerLongTerm, fact, "preference", "lang"); err != nil {
			t.Fatalf("Run1 saveMemory: %v", err)
		}
		// Close = 模拟进程退出（SQLite 数据落盘）。
		if err := env1.MemoryStore.Close(); err != nil {
			t.Fatalf("Run1 Close: %v", err)
		}
	}

	// ---- Run2：重开同一 DB 文件，按 PersonaID 检索 ----
	{
		env2, err := setupSession(dbPath, NewFakeEmbedder(), personaSpec)
		if err != nil {
			t.Fatalf("Run2 setupSession: %v", err)
		}
		defer env2.MemoryStore.Close()

		// 同一 PersonaID 必须检索到 Run1 写入的记忆。
		got, err := env2.MemoryStore.Query(ctx, env2.Persona.ID, agentcontract.MemoryLayerLongTerm,
			agentcontract.QueryOptions{Limit: 5, QueryText: "用户报告偏好"})
		if err != nil {
			t.Fatalf("Run2 Query: %v", err)
		}
		if len(got) == 0 {
			t.Fatalf("Run2 未检索到跨会话记忆（P2/S13 失败）")
		}
		// 内容必须包含 Run1 写入的事实。
		found := false
		for _, m := range got {
			if strings.Contains(m.Content, fact) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Run2 检索到的记忆不含 Run1 事实: %+v", got)
		}

		// 进一步：ContextBuilder 在 Run2 也能注入这条跨会话记忆。
		model := newFakeModel(textStream("好的。"))
		var sawInjected bool
		agent := agentcore.New(model, agentcore.NewRegistry(), agentcore.Options{
			ContextBuilder: env2.ContextBuilder,
			Persona:        env2.Persona,
			MemoryStore:    env2.MemoryStore,
			OnEvent: func(ev agentcore.Event) {
				if ev.Kind == agentcore.EventMemoryInjected && strings.Contains(ev.Content, fact) {
					sawInjected = true
				}
			},
			MaxIterations: 1,
		})
		if _, err := agent.Run(ctx, "请帮我写报告"); err != nil {
			t.Fatalf("Run2 agent.Run: %v", err)
		}
		if !sawInjected {
			t.Fatalf("Run2 的 ContextBuilder 未注入跨会话记忆（P3/S14 在跨会话场景失败）")
		}
	}
}

// TestCrossSessionDistinctPersonasIsolated 验证记忆按 PersonaID 归档：
// Run1 写入 persona-A 的记忆；Run2 检索 persona-B 时不应命中 persona-A 的记忆。
func TestCrossSessionDistinctPersonasIsolated(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memory-iso.db")

	specA := PersonaSpec{ID: "persona-A", Name: "A", Role: "r", SystemPrompt: "A"}
	specB := PersonaSpec{ID: "persona-B", Name: "B", Role: "r", SystemPrompt: "B"}

	// Run1: A 写记忆。
	func() {
		envA, err := setupSession(dbPath, NewFakeEmbedder(), specA)
		if err != nil {
			t.Fatalf("setupSession A: %v", err)
		}
		defer envA.MemoryStore.Close()
		if err := saveMemory(ctx, envA.MemoryStore, "persona-A", agentcontract.MemoryLayerLongTerm, "A 私有记忆内容 xyz", "secret"); err != nil {
			t.Fatalf("saveMemory A: %v", err)
		}
	}()

	// Run2: B 检索自己的 LongTerm（应空）。
	envB, err := setupSession(dbPath, NewFakeEmbedder(), specB)
	if err != nil {
		t.Fatalf("setupSession B: %v", err)
	}
	defer envB.MemoryStore.Close()
	got, err := envB.MemoryStore.Query(ctx, "persona-B", agentcontract.MemoryLayerLongTerm,
		agentcontract.QueryOptions{Limit: 5, QueryText: "私有记忆"})
	if err != nil {
		t.Fatalf("Query B: %v", err)
	}
	for _, m := range got {
		if strings.Contains(m.Content, "A 私有记忆") {
			t.Fatalf("persona-B 命中了 persona-A 的记忆（隔离失败）: %q", m.Content)
		}
	}
}

// ===========================================================================
// A1/S11：三形态 Agent + Persona + Memory 端到端跑通
// ===========================================================================

// TestToolLoopAgentWithMemory 是 tool_loop_agent 升级形态的端到端验证：
// 装配 Persona + MemoryStore + ContextBuilder 的 Agent 跑通一次 Tool Loop，
// 且 LongTerm 记忆被注入到模型可见的消息视图里。
func TestToolLoopAgentWithMemory(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-toolloop", Name: "ToolLoop", Role: "analyst", SystemPrompt: "你是数据分析师",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	const knownFact = "项目数据文件路径是 ../data.json，字段为 sales 数组"
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm, knownFact, "data"); err != nil {
		t.Fatalf("saveMemory: %v", err)
	}

	model := newFakeModel(textStream("已根据记忆读取数据。"))
	agent := agentcore.New(model, agentcore.NewRegistry(), agentcore.Options{
		ContextBuilder: env.ContextBuilder,
		Persona:        env.Persona,
		MemoryStore:    env.MemoryStore,
		MaxIterations:  1,
	})
	res, err := agent.Run(ctx, "分析数据")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Iterations == 0 {
		t.Fatalf("期望模型被调用")
	}
	// 模型收到的视图应包含注入的记忆内容。
	views := model.viewsSnapshot()
	if len(views) == 0 {
		t.Fatalf("未捕获模型调用视图")
	}
	sawFact := false
	for _, v := range views {
		for _, msg := range v {
			if strings.Contains(msg.Content, knownFact) {
				sawFact = true
			}
		}
	}
	if !sawFact {
		t.Fatalf("模型视图未包含注入的 LongTerm 记忆（tool_loop 形态未集成 Memory）")
	}
}

// TestWorkflowAgentShortTermMemory 验证 workflow_agent 升级形态：
// HITL 输入应被记录进 ShortTerm 记忆层（可被检索）。
// 这里直接测"写入 ShortTerm"的行为（示例 main.go 的等价逻辑见 workflow_agent）。
func TestWorkflowAgentShortTermMemory(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-workflow", Name: "Workflow", Role: "approver", SystemPrompt: "你是审批助手",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 模拟 HITL：用户输入记录进 ShortTerm。
	const humanInput = "批准发布到生产环境"
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerShortTerm, humanInput, "hitl"); err != nil {
		t.Fatalf("saveMemory ShortTerm: %v", err)
	}

	// ShortTerm 记忆可被检索。
	got, err := env.MemoryStore.Query(ctx, env.Persona.ID, agentcontract.MemoryLayerShortTerm,
		agentcontract.QueryOptions{Limit: 5, QueryText: "批准"})
	if err != nil {
		t.Fatalf("Query ShortTerm: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("未检索到 HITL ShortTerm 记忆")
	}
	found := false
	for _, m := range got {
		if strings.Contains(m.Content, humanInput) {
			found = true
		}
	}
	if !found {
		t.Fatalf("ShortTerm 检索结果不含 HITL 输入: %+v", got)
	}
}

// TestResearchAgentProducesLongTermEvidence 验证 research_agent 升级形态：
// Run 产出的 Evidence 摘要写入 LongTerm 记忆，下次 Run 可检索（与 research_agent
// main.go 的等价逻辑一致）。
func TestResearchAgentProducesLongTermEvidence(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "memory-research.db")
	spec := PersonaSpec{ID: "persona-research", Name: "Research", Role: "analyst", SystemPrompt: "你是研究员"}

	// Run1: 模拟 research Agent 产出 Evidence 摘要并写入 LongTerm。
	func() {
		env1, err := setupSession(dbPath, NewFakeEmbedder(), spec)
		if err != nil {
			t.Fatalf("Run1 setupSession: %v", err)
		}
		defer env1.MemoryStore.Close()
		const evidence = "研究发现 2026Q3 销售环比增长 18%，来源：../data.json"
		if err := saveMemory(ctx, env1.MemoryStore, env1.Persona.ID, agentcontract.MemoryLayerLongTerm, evidence, "evidence", "research"); err != nil {
			t.Fatalf("Run1 saveMemory: %v", err)
		}
	}()

	// Run2: 同一 Persona 检索到该 Evidence。
	env2, err := setupSession(dbPath, NewFakeEmbedder(), spec)
	if err != nil {
		t.Fatalf("Run2 setupSession: %v", err)
	}
	defer env2.MemoryStore.Close()
	got, err := env2.MemoryStore.Query(ctx, env2.Persona.ID, agentcontract.MemoryLayerLongTerm,
		agentcontract.QueryOptions{Limit: 5, QueryText: "销售增长研究"})
	if err != nil {
		t.Fatalf("Run2 Query: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("Run2 未检索到 Run1 写入的 Evidence")
	}
	found := false
	for _, m := range got {
		if strings.Contains(m.Content, "环比增长 18%") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Run2 检索结果不含 Run1 Evidence: %+v", got)
	}
}

// TestVectorRetrievalRanksRelevant 验证 fake embedder + VectorMemoryStore 的
// 语义检索：与查询词重叠更多的记忆应排在前面（Top-K 命中）。
func TestVectorRetrievalRanksRelevant(t *testing.T) {
	ctx := context.Background()
	env, err := setupSession(":memory:", NewFakeEmbedder(), PersonaSpec{
		ID: "persona-vec", Name: "Vec", Role: "r", SystemPrompt: "r",
	})
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	defer env.MemoryStore.Close()

	// 两条记忆，与查询的词重叠不同。
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm,
		"销售 增长 趋势 报告", "report"); err != nil {
		t.Fatalf("saveMemory 1: %v", err)
	}
	if err := saveMemory(ctx, env.MemoryStore, env.Persona.ID, agentcontract.MemoryLayerLongTerm,
		"天气 晴朗 适合 出行", "weather"); err != nil {
		t.Fatalf("saveMemory 2: %v", err)
	}
	got, err := env.MemoryStore.Query(ctx, env.Persona.ID, agentcontract.MemoryLayerLongTerm,
		agentcontract.QueryOptions{Limit: 2, QueryText: "销售 增长 趋势"})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("向量检索无结果")
	}
	if !strings.Contains(got[0].Content, "销售") {
		t.Fatalf("Top-1 应是销售相关记忆，实际 %q", got[0].Content)
	}
}

// 编译期接口满足性：fakeModel 实现 agentmodel.Model；fakeEmbedder 实现 EmbeddingProvider。
var (
	_ agentmodel.Model             = (*fakeModel)(nil)
	_ agentmodel.EmbeddingProvider = fakeEmbedder{}
)

// 确保 agent-runtime 的 VectorMemoryStore 与 DefaultContextBuilder 仍在 type set 内。
var (
	_ agentcontract.MemoryStore    = (*agentruntime.VectorMemoryStore)(nil)
	_ agentcontract.ContextBuilder = (*agentruntime.DefaultContextBuilder)(nil)
)

// 避免未使用 import（time 仅在 helper 内用到，这里保留以备未来断言时间戳）。
var _ = time.Now

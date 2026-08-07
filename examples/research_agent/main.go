// Command research_agent 是 Research Pattern 验证示例（v2.0 升级，Issue #46）：
// 用 agent-runtime 的 ResearchPattern（多轮研究 → 报告 Artifact + Evidence →
// Evaluator 验收），并加载 Persona + MemoryStore + ContextBuilder（A1/A2/S11/P1）。
//
// v2.0 新增：研究产出的 Evidence 摘要写入 LongTerm 记忆，下次 Run 可检索
// （A3/S13/P2 跨会话恢复的写入端）。
//
// 运行（在 examples/research_agent 目录）：
//
//	go run . --task "读取 ../data.json，分析销售趋势并输出报告"
//
// 启用记忆持久化（下次运行会注入历史研究结论）：
//
//	go run . --persona-id researcher --memory ./mem.db --embed-dim 1536
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	modelopenai "github.com/mantis-layer/mts/adapters/model-openai"
	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentcore "github.com/mantis-layer/mts/agent-core"
	agentruntime "github.com/mantis-layer/mts/agent-runtime"
	"github.com/mantis-layer/mts/tools"
)

func main() {
	loadEnvFile(".env.local")

	baseURL := flag.String("baseurl", "", "OpenAI 兼容 baseurl")
	apiKey := flag.String("api-key", "", "API key")
	model := flag.String("model", "", "模型名")
	task := flag.String("task", "", "研究任务描述")
	// v2.0 Persona + Memory 抽象（A1/A2/S11）。
	personaID := flag.String("persona-id", "research-demo", "Persona ID（跨会话身份锚点）")
	personaName := flag.String("persona-name", "研究员", "Persona 名称")
	memoryPath := flag.String("memory", "", "记忆 SQLite 文件路径（空=不启用记忆注入与持久化）")
	embedDim := flag.Int("embed-dim", 1536, "embedding 维度（需与所选 embedding 模型一致）")
	flag.Parse()

	bURL := firstNonEmpty(*baseURL, os.Getenv("MTS_BASEURL"))
	aKey := firstNonEmpty(*apiKey, os.Getenv("MTS_API_KEY"))
	mName := firstNonEmpty(*model, os.Getenv("MTS_MODEL"))
	if bURL == "" || aKey == "" || mName == "" {
		log.Fatal("需要 MTS_BASEURL / MTS_API_KEY / MTS_MODEL（或对应 flag）")
	}

	// 1. 模型 + 检索 Agent（只带读取类工具）
	client, err := modelopenai.New(modelopenai.Config{BaseURL: bURL, APIKey: aKey, Model: mName})
	if err != nil {
		log.Fatalf("初始化模型: %v", err)
	}

	// 2. Persona + MemoryStore + ContextBuilder（v2.0 三件套，A1/A2/S11/P1）。
	persona := &agentcontract.Persona{
		ID:           *personaID,
		Name:         *personaName,
		Role:         "researcher",
		SystemPrompt: "你是一名严谨的研究员，基于数据来源产出有据可查的研究报告。",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := persona.Validate(); err != nil {
		log.Fatalf("Persona 校验: %v", err)
	}

	var memStore *agentruntime.VectorMemoryStore
	var ctxBuilder agentcontract.ContextBuilder
	if *memoryPath != "" {
		memStore, err = agentruntime.NewVectorMemoryStore(*memoryPath, client, *embedDim)
		if err != nil {
			log.Fatalf("创建 MemoryStore: %v", err)
		}
		defer memStore.Close()
		ctxBuilder = agentruntime.NewDefaultContextBuilder()
	}

	// 3. 组装 Agent（Options 装配三件套；MemoryStore 为 nil 时 agent-core 自动跳过）。
	reg := agentcore.NewRegistry()
	if err := reg.Register(tools.FileReader{}); err != nil {
		log.Fatalf("注册工具: %v", err)
	}
	agent := agentcore.New(client, reg, agentcore.Options{
		ContextBuilder: ctxBuilder,
		Persona:        persona,
		MemoryStore:    memStore,
		OnEvent: func(ev agentcore.Event) {
			if ev.Kind == agentcore.EventMemoryInjected && ev.Content != "" {
				fmt.Printf("[记忆注入] 检索到历史研究结论\n")
			}
		},
	})

	// 4. Runtime + Research Pattern + Evaluator
	rt, err := agentruntime.NewRuntime(
		agentruntime.NewMemoryStorage(),
		agentruntime.Budget{MaxIterations: 8, MaxToolCalls: 20},
	)
	if err != nil {
		log.Fatalf("创建 Runtime: %v", err)
	}
	defer rt.Close()
	if err := rt.RegisterPattern(agentruntime.NewResearchPattern(agent)); err != nil {
		log.Fatalf("注册 ResearchPattern: %v", err)
	}
	if err := rt.RegisterEvaluator(&agentruntime.EvidenceCoverageEvaluator{Required: 1}); err != nil {
		log.Fatalf("注册 Evaluator: %v", err)
	}

	// 5. 提交研究任务
	input := *task
	if input == "" {
		input = "读取 ../data.json，分析销售数据趋势并输出中文研究报告"
	}
	run, err := rt.SubmitTask(context.Background(), &agentruntime.Task{
		ID:        "research-demo-" + time.Now().Format("20060102150405"),
		Name:      "research-demo",
		Pattern:   "research",
		Input:     input,
		PersonaID: persona.ID,
	})
	if err != nil {
		log.Fatalf("提交任务: %v", err)
	}
	fmt.Printf("已提交任务: %s (run=%s, persona=%s)\n", run.TaskID, run.ID, persona.ID)

	// 6. 执行并轮询（展示事件与证据）
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := rt.Run(ctx, run.ID)
		done <- err
	}()
	var printed int
	for {
		cur, err := rt.GetRun(context.Background(), run.ID)
		if err != nil {
			log.Fatalf("查询运行: %v", err)
		}
		if len(cur.Summary) > printed {
			fmt.Printf("[%s] %s", cur.State, cur.Summary[printed:])
			printed = len(cur.Summary)
		}
		switch cur.State {
		case agentruntime.RunStateCompleted, agentruntime.RunStateFailed, agentruntime.RunStateCancelled:
			goto finish
		default:
			time.Sleep(500 * time.Millisecond)
		}
	}
finish:
	if err := <-done; err != nil {
		log.Fatalf("运行失败: %v", err)
	}

	// 7. 结果：报告 Artifact + Evaluator 验收
	final, err := rt.GetRun(context.Background(), run.ID)
	if err != nil {
		log.Fatalf("查询终态: %v", err)
	}
	fmt.Printf("\n=== 终态 ===\n状态: %s\n摘要: %s\n", final.State, final.Summary)
	if final.State != agentruntime.RunStateCompleted || final.Result == nil {
		if final.Error != "" {
			fmt.Printf("错误: %s\n", final.Error)
		}
		return
	}
	for _, a := range final.Result.Artifacts {
		fmt.Printf("Artifact: %s (type=%s, id=%s)\n%s\n", a.Name, a.Type, a.ID, a.Content)
	}
	evs, _ := rt.Events(context.Background(), run.ID)
	for _, ev := range evs {
		if ev.Kind == agentruntime.EventEvaluatorResult {
			fmt.Printf("Evaluator: %v\n", ev.Data)
		}
	}

	// 8. v2.0：把研究结论写入 LongTerm 记忆（下次 Run 可检索，A3/S13/P2 写入端）。
	if memStore != nil && final.Result != nil && len(final.Result.Artifacts) > 0 {
		report := final.Result.Artifacts[0].Content
		if err := saveEvidence(context.Background(), memStore, persona.ID, report); err != nil {
			log.Printf("写入研究结论到记忆失败: %v", err)
		} else {
			fmt.Printf("[记忆] 已将本次研究结论写入 LongTerm（persona=%s），下次运行可注入\n", persona.ID)
		}
	}

	fmt.Printf("=== 统计 ===\n工具调用: %d | 轮次: %d | tokens: %d\n",
		final.ToolCalls, final.Iterations, final.Usage.TotalTokens)
}

// saveEvidence 把研究报告摘要作为 LongTerm 记忆持久化（跨会话可检索）。
func saveEvidence(ctx context.Context, store *agentruntime.VectorMemoryStore, personaID, report string) error {
	m := &agentcontract.Memory{
		ID:        fmt.Sprintf("%s-research-%d", personaID, time.Now().UnixNano()),
		PersonaID: personaID,
		Layer:     agentcontract.MemoryLayerLongTerm,
		Content:   clip(report, 800), // 摘要长度上限，避免记忆膨胀
		Tags:      []string{"research", "evidence"},
		CreatedAt: time.Now(),
	}
	return store.Save(ctx, m)
}

// clip 截断超长文本。
func clip(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return strings.TrimSpace(string([]rune(s)[:n])) + "…"
}

// firstNonEmpty 返回首个非空值。
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// loadEnvFile 从当前目录向上查找 .env.local 并加载（不覆盖已有 env）。
func loadEnvFile(name string) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for i := 0; i < 4; i++ {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				key, value, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				key = strings.TrimSpace(key)
				value = strings.Trim(strings.TrimSpace(value), `"'`)
				if os.Getenv(key) == "" {
					_ = os.Setenv(key, value)
				}
			}
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

var _ = agentcore.EventToolStart // 保持 agent-core 引用（依赖方向示例）

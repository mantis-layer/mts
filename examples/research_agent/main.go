// Command research_agent 是 Research Pattern 验证示例：用 agent-runtime 的
// ResearchPattern（多轮研究 → 报告 Artifact + Evidence → Evaluator 验收）。
//
// 运行（在 examples/research_agent 目录）：
//
//	go run . --task "读取 ../data.json，分析销售趋势并输出报告"
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
	"github.com/mantis-layer/mts/agent-compose"
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
	agent, err := agentcompose.NewBuilder().
		Name("research-demo").
		Model(client).
		Tools(tools.FileReader{}).
		Build()
	if err != nil {
		log.Fatalf("组装 Agent: %v", err)
	}

	// 2. Runtime + Research Pattern + Evaluator
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

	// 3. 提交研究任务
	input := *task
	if input == "" {
		input = "读取 ../data.json，分析销售数据趋势并输出中文研究报告"
	}
	run, err := rt.SubmitTask(context.Background(), &agentruntime.Task{
		ID:   "research-demo-" + time.Now().Format("20060102150405"),
		Name: "research-demo", Pattern: "research", Input: input,
	})
	if err != nil {
		log.Fatalf("提交任务: %v", err)
	}
	fmt.Printf("已提交任务: %s (run=%s)\n", run.TaskID, run.ID)

	// 4. 执行并轮询（展示事件与证据）
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

	// 5. 结果：报告 Artifact + Evaluator 验收（失败/取消时 Result 为空）
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
	fmt.Printf("=== 统计 ===\n工具调用: %d | 轮次: %d | tokens: %d\n",
		final.ToolCalls, final.Iterations, final.Usage.TotalTokens)
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

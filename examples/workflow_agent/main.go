// Command workflow_agent 是 Workflow Pattern 验证示例（v2.0 升级，Issue #46）：
// 多步骤工作流（读取 → 人工审批 → 汇总），展示 agent-runtime 的人工暂停/恢复。
//
// v2.0 新增：加载 Persona + MemoryStore（A1/A2/S11/P1），并把 HITL 人工输入
// 记录进 ShortTerm 记忆（跨会话保留审批历史）。
//
// 运行（在 examples/workflow_agent 目录）：
//
//	go run . --task "读取 ../data.json 并按月汇总" --approve
//
// 启用记忆持久化（记录审批历史）：
//
//	go run . --persona-id approver --memory ./mem.db --approve
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentruntime "github.com/mantis-layer/mts/agent-runtime"
	"github.com/mantis-layer/mts/tools"
)

func main() {
	loadEnvFile(".env.local")

	task := flag.String("task", "", "工作流任务描述")
	autoApprove := flag.Bool("approve", false, "自动批准人工审批节点（非交互）")
	// v2.0 Persona + Memory 抽象（A1/A2/S11）。
	personaID := flag.String("persona-id", "workflow-demo", "Persona ID（跨会话身份锚点）")
	personaName := flag.String("persona-name", "审批助手", "Persona 名称")
	memoryPath := flag.String("memory", "", "记忆 SQLite 文件路径（空=不记录审批历史）")
	embedDim := flag.Int("embed-dim", 1536, "embedding 维度（启用了 memory 但无 embedding provider 时可传 16）")
	flag.Parse()

	// 1. Persona + MemoryStore（v2.0 三件套的子集，A1/A2/S11/P1）。
	//    workflow 形态无需模型，故不构造 ContextBuilder（注入由模型驱动的形态使用）；
	//    这里用 MemoryStore 记录 HITL 输入到 ShortTerm 层。
	persona := &agentcontract.Persona{
		ID:           *personaID,
		Name:         *personaName,
		Role:         "approver",
		SystemPrompt: "你是一名负责审批的工作流助手。",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	if err := persona.Validate(); err != nil {
		log.Fatalf("Persona 校验: %v", err)
	}

	var memStore *agentruntime.VectorMemoryStore
	if *memoryPath != "" {
		// workflow 形态无 embedding provider：传 nil，VectorMemoryStore 退化为规则检索
		// （ShortTerm 按时间倒序检索，足以支撑审批历史场景）。
		var err error
		memStore, err = agentruntime.NewVectorMemoryStore(*memoryPath, nil, *embedDim)
		if err != nil {
			log.Fatalf("创建 MemoryStore: %v", err)
		}
		defer memStore.Close()
	}

	// 2. 组装 Workflow：三个步骤（读取 → 人工审批 → 汇总）
	fileReader := tools.FileReader{}
	steps := []agentruntime.WorkflowStep{
		{
			Name: "读取数据",
			Action: func(ctx context.Context, input string) (string, error) {
				out, err := fileReader.Execute(ctx, map[string]any{"path": "../data.json"})
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("已读取数据文件（%v 字节）", len(fmt.Sprint(out["content"]))), nil
			},
		},
		{
			Name:   "人工审批",
			Human:  true,
			Prompt: "是否批准继续执行？(输入 批准/yes 继续，拒绝/no 终止)",
		},
		{
			Name: "汇总计算",
			Action: func(ctx context.Context, input string) (string, error) {
				out, err := fileReader.Execute(ctx, map[string]any{"path": "../data.json"})
				if err != nil {
					return "", err
				}
				content := fmt.Sprint(out["content"])
				return fmt.Sprintf("数据文件已读取（%d 字节，含 %d 条记录）",
					len(content), strings.Count(content, "\"id\"")), nil
			},
		},
	}

	// 3. Runtime + Workflow Pattern
	rt, err := agentruntime.NewRuntime(
		agentruntime.NewMemoryStorage(),
		agentruntime.Budget{MaxIterations: 10},
	)
	if err != nil {
		log.Fatalf("创建 Runtime: %v", err)
	}
	defer rt.Close()
	if err := rt.RegisterPattern(agentruntime.NewWorkflowPattern(steps)); err != nil {
		log.Fatalf("注册 WorkflowPattern: %v", err)
	}

	// 4. 提交并运行
	run, err := rt.SubmitTask(context.Background(), &agentruntime.Task{
		ID:        "workflow-demo-" + time.Now().Format("20060102150405"),
		Name:      "workflow-demo",
		Pattern:   "workflow",
		Input:     *task,
		PersonaID: persona.ID,
	})
	if err != nil {
		log.Fatalf("提交任务: %v", err)
	}
	fmt.Printf("已提交任务: %s (run=%s, persona=%s)\n", run.TaskID, run.ID, persona.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() {
		// 忽略预期的 waiting 错误；真实错误（storage 故障等）会体现在终态。
		_, _ = rt.Run(ctx, run.ID)
	}()

	// 5. 轮询：WAITING_HUMAN 时收集人工输入、提交、并记录到 ShortTerm 记忆（v2.0）。
	reader := bufio.NewReader(os.Stdin)
	var lastProgress string
	for {
		cur, err := rt.GetRun(context.Background(), run.ID)
		if err != nil {
			log.Fatalf("查询运行: %v", err)
		}
		if cur.Progress != "" && cur.Progress != lastProgress {
			fmt.Printf("[步骤] %s\n", cur.Progress)
			lastProgress = cur.Progress
		}
		switch cur.State {
		case agentruntime.RunStateWaiting:
			// 找到审批提示（从事件流取）
			prompt := "是否批准继续？(yes/no)"
			evs, _ := rt.Events(context.Background(), run.ID)
			for _, ev := range evs {
				if ev.Kind == agentruntime.EventHumanInputRequested {
					if p, ok := ev.Data["prompt"].(string); ok && p != "" {
						prompt = p
					}
				}
			}
			fmt.Printf("⏸ 等待人工输入：%s\n", prompt)
			var answer string
			if *autoApprove {
				answer = "yes"
				fmt.Println("（--approve 自动批准）")
			} else {
				line, _ := reader.ReadString('\n')
				answer = strings.TrimSpace(line)
			}
			if answer == "" {
				fmt.Println("（输入为空，请重新输入）")
				continue
			}
			// v2.0：把 HITL 输入记录进 ShortTerm 记忆（A1/A2/S11/P1，跨会话保留审批历史）。
			if memStore != nil {
				if err := saveHumanInput(context.Background(), memStore, persona.ID, prompt, answer); err != nil {
					log.Printf("记录 HITL 输入到 ShortTerm 失败: %v", err)
				}
			}
			if _, err := rt.SubmitHumanInput(context.Background(), run.ID, answer); err != nil {
				log.Fatalf("提交人工输入: %v", err)
			}
		case agentruntime.RunStateCompleted, agentruntime.RunStateFailed, agentruntime.RunStateCancelled:
			goto finish
		default:
			time.Sleep(300 * time.Millisecond)
		}
	}
finish:
	final, _ := rt.GetRun(context.Background(), run.ID)
	fmt.Printf("\n=== 终态 ===\n状态: %s\n摘要: %s\n进度: %s\n",
		final.State, final.Summary, final.Progress)
	if final.State == agentruntime.RunStateFailed {
		fmt.Printf("错误: %v\n", final.Error)
	}
	if memStore != nil {
		fmt.Printf("[记忆] HITL 输入已记录到 ShortTerm（persona=%s）\n", persona.ID)
	}
}

// saveHumanInput 把一次 HITL 输入记录进 ShortTerm 记忆层。
func saveHumanInput(ctx context.Context, store *agentruntime.VectorMemoryStore, personaID, prompt, answer string) error {
	m := &agentcontract.Memory{
		ID:        fmt.Sprintf("%s-hitl-%d", personaID, time.Now().UnixNano()),
		PersonaID: personaID,
		Layer:     agentcontract.MemoryLayerShortTerm,
		Content:   fmt.Sprintf("审批提问：%s | 用户决定：%s", prompt, answer),
		Tags:      []string{"hitl", "approval"},
		CreatedAt: time.Now(),
	}
	return store.Save(ctx, m)
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

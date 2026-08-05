// Command workflow_agent 是 Workflow Pattern 验证示例：多步骤工作流
// （读取 → 人工审批 → 汇总），展示 agent-runtime 的人工暂停/恢复。
//
// 运行（在 examples/workflow_agent 目录）：
//
//	go run . --task "读取 ../data.json 并按月汇总"
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

	agentruntime "github.com/mantis-layer/mts/agent-runtime"
	"github.com/mantis-layer/mts/tools"
)

func main() {
	loadEnvFile(".env.local")

	task := flag.String("task", "", "工作流任务描述")
	autoApprove := flag.Bool("approve", false, "自动批准人工审批节点（非交互）")
	flag.Parse()

	// 1. 组装 Workflow：三个步骤（读取 → 人工审批 → 汇总）
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

	// 2. Runtime + Workflow Pattern
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

	// 3. 提交并运行
	run, err := rt.SubmitTask(context.Background(), &agentruntime.Task{
		ID:   "workflow-demo-" + time.Now().Format("20060102150405"),
		Name: "workflow-demo", Pattern: "workflow", Input: *task,
	})
	if err != nil {
		log.Fatalf("提交任务: %v", err)
	}
	fmt.Printf("已提交任务: %s (run=%s)\n", run.TaskID, run.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	go func() { _, _ = rt.Run(ctx, run.ID) }()

	// 4. 轮询：WAITING_HUMAN 时收集人工输入并提交
	reader := bufio.NewReader(os.Stdin)
	for {
		cur, err := rt.GetRun(context.Background(), run.ID)
		if err != nil {
			log.Fatalf("查询运行: %v", err)
		}
		if cur.Progress != "" {
			fmt.Printf("[步骤] %s\n", cur.Progress)
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

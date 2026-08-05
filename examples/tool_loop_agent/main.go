// Command tool_loop_agent 是 PRD S10 的验证示例：不改 agent-core 源码，
// 用 Go API 在 300 行内创建并跑通一个 Tool Loop Agent。
//
// 运行（在 examples/tool_loop_agent 目录）：
//
//	go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"
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

	modelopenai "github.com/mantis-layer/mts/adapters/model-openai"
	"github.com/mantis-layer/mts/agent-compose"
	"github.com/mantis-layer/mts/agent-core"
	"github.com/mantis-layer/mts/tools"
)

func main() {
	loadEnvFile(".env.local")

	baseURL := flag.String("baseurl", "", "OpenAI 兼容 baseurl")
	apiKey := flag.String("api-key", "", "API key")
	model := flag.String("model", "", "模型名")
	task := flag.String("task", "", "任务描述；为空时读取 stdin")
	flag.Parse()

	bURL := firstNonEmpty(*baseURL, os.Getenv("MTS_BASEURL"))
	aKey := firstNonEmpty(*apiKey, os.Getenv("MTS_API_KEY"))
	mName := firstNonEmpty(*model, os.Getenv("MTS_MODEL"))
	if bURL == "" || aKey == "" || mName == "" {
		log.Fatal("需要 MTS_BASEURL / MTS_API_KEY / MTS_MODEL（或对应 flag）")
	}

	// 1. 模型
	client, err := modelopenai.New(modelopenai.Config{BaseURL: bURL, APIKey: aKey, Model: mName})
	if err != nil {
		log.Fatalf("初始化模型: %v", err)
	}

	// 2. 组装 Agent（Go Builder API，G5）
	agent, err := agentcompose.NewBuilder().
		Name("tool-loop-demo").
		Model(client).
		Tools(tools.FileReader{}, tools.Calculator{}).
		OnEvent(func(ev agentcore.Event) {
			switch ev.Kind {
			case agentcore.EventToolStart:
				fmt.Printf("[工具] %s\n", ev.Tool)
			case agentcore.EventModelDelta:
				fmt.Print(ev.Content)
			}
		}).
		Build()
	if err != nil {
		log.Fatalf("组装 Agent: %v", err)
	}

	// 3. 任务输入
	input := *task
	if input == "" {
		fmt.Print("任务：")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			log.Fatal("无任务输入")
		}
		input = strings.TrimSpace(line)
	}

	// 4. 运行
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := agent.Run(ctx, input)
	if err != nil {
		log.Fatalf("运行失败: %v", err)
	}

	// 5. 结果
	fmt.Printf("\n=== 结果 ===\n%s\n", res.FinalMessage.Content)
	fmt.Printf("=== 统计 ===\n工具调用: %d | 模型轮次: %d | tokens: %d\n",
		res.ToolCalls, res.Iterations, res.Usage.TotalTokens)
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

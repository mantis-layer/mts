// Command mts 是最小 Agent CLI（FR-008）：通过 OpenAI 兼容端点
// 配置（MTS_BASEURL/MTS_API_KEY/MTS_MODEL 或对应 flag）运行 Tool Loop Agent。
//
// 示例：
//
//	mts --task "读取 data.json，用 calculator 计算销量总和，输出摘要"
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	modelopenai "github.com/mantis-layer/mts/adapters/model-openai"
	"github.com/mantis-layer/mts/agent-core"
	"github.com/mantis-layer/mts/tools"
)

func main() {
	// 先加载本地 .env.local（若存在），再叠加 flag / 环境变量。
	loadEnvFile(".env.local")

	var (
		// flag 默认值保持空，避免 --help 泄露密钥；env 值在解析后回退读取。
		baseURL = flag.String("baseurl", "", "OpenAI 兼容端点 baseurl（或 MTS_BASEURL）")
		apiKey  = flag.String("api-key", "", "API key（或 MTS_API_KEY）")
		model   = flag.String("model", "", "模型名称（或 MTS_MODEL）")
		task    = flag.String("task", "", "任务描述；为空时读取 stdin")
		jsonOut = flag.Bool("json", false, "以 JSON Lines 输出事件")
	)
	flag.Parse()

	if *baseURL == "" {
		*baseURL = os.Getenv("MTS_BASEURL")
	}
	if *apiKey == "" {
		*apiKey = os.Getenv("MTS_API_KEY")
	}
	if *model == "" {
		*model = os.Getenv("MTS_MODEL")
	}

	if *baseURL == "" || *apiKey == "" || *model == "" {
		log.Fatal("需要配置 MTS_BASEURL、MTS_API_KEY、MTS_MODEL（或 --baseurl/--api-key/--model）")
	}

	client, err := modelopenai.New(modelopenai.Config{BaseURL: *baseURL, APIKey: *apiKey, Model: *model})
	if err != nil {
		log.Fatalf("初始化模型失败: %v", err)
	}

	registry := agentcore.NewRegistry()
	if err := registry.Register(tools.FileReader{}); err != nil {
		log.Fatalf("注册 file_reader 失败: %v", err)
	}
	if err := registry.Register(tools.Calculator{}); err != nil {
		log.Fatalf("注册 calculator 失败: %v", err)
	}

	agent := agentcore.New(client, registry, agentcore.Options{
		OnEvent: func(ev agentcore.Event) { printEvent(ev, *jsonOut) },
	})

	input := *task
	if input == "" {
		fmt.Print("请输入任务描述：")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			log.Fatal("未提供任务输入")
		}
		input = strings.TrimSpace(line)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	res, err := agent.Run(ctx, input)
	if err != nil {
		if ctx.Err() != nil {
			fmt.Fprintf(os.Stderr, "\n[运行被取消]\n")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "\n[运行失败] %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n=== 最终结果 ===\n%s\n", res.FinalMessage.Content)
	fmt.Printf("=== Usage ===\n输入 tokens: %d | 输出 tokens: %d | 总计: %d\n", res.Usage.PromptTokens, res.Usage.CompletionTokens, res.Usage.TotalTokens)
	fmt.Printf("=== 统计 ===\n工具调用: %d | 模型轮次: %d\n", res.ToolCalls, res.Iterations)
}

func printEvent(ev agentcore.Event, jsonOut bool) {
	if jsonOut {
		line := map[string]any{
			"kind":      ev.Kind,
			"timestamp": ev.Timestamp,
			"model":     ev.Model,
			"tool":      ev.Tool,
			"content":   ev.Content,
			"usage":     ev.Usage,
		}
		if ev.Error != nil {
			line["error"] = ev.Error.Error()
		}
		b, _ := json.Marshal(line)
		fmt.Println(string(b))
		return
	}
	switch ev.Kind {
	case agentcore.EventModelDelta:
		fmt.Print(ev.Content)
	case agentcore.EventToolStart:
		fmt.Printf("\n[工具调用] %s (%s)\n", ev.Tool, ev.ToolCallID)
	case agentcore.EventToolDone:
		fmt.Printf("[工具结果] %s → %s\n", ev.Tool, truncate(ev.Content, 200))
	case agentcore.EventToolError:
		fmt.Printf("[工具错误] %s → %v\n", ev.Tool, ev.Error)
	case agentcore.EventAgentError:
		fmt.Printf("[Agent 错误] %v\n", ev.Error)
	case agentcore.EventAgentMessage:
		fmt.Printf("\n[Agent 完成]\n")
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// loadEnvFile 从当前目录向上查找 KEY=VALUE 环境文件（不覆盖已存在 env），
// 并在仓库根（含 .git 或 go.work）停止，防止读取仓库外祖先目录的
// .env.local（凭证钓鱼向量，NFR-004）。
func loadEnvFile(name string) {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, name)
		if data, err := os.ReadFile(candidate); err == nil {
			loadEnvLines(string(data))
			return
		}
		if isRepoRoot(dir) {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}

// isRepoRoot 判断目录是否为仓库根（含 .git 或 go.work；module 子目录的
// go.mod 不计，monorepo 根以 go.work 为特征）。
func isRepoRoot(dir string) bool {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
		return true
	}
	return false
}

// loadEnvLines 解析 KEY=VALUE 行（支持可选的 export 前缀与引号）。
func loadEnvLines(data string) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "export ")
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
		if key != "" && os.Getenv(key) == "" {
			_ = os.Setenv(key, value)
		}
	}
}

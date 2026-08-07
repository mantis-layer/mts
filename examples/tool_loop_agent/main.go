// Command tool_loop_agent 是 PRD S10 / v2.0 Issue #46 的验证示例：
// 用 agent-core 跑通一个 Tool Loop Agent，并升级为
// **加载 Persona + MemoryStore + ContextBuilder**（A1/A2/S11/P1）——
// 每次模型调用前注入 Persona 的 LongTerm 记忆（A4/S14/P3）。
//
// 不修改 agent-core 源码，仅通过 Options 装配身份/记忆/注意力三件套。
// 与 research_agent / workflow_agent 共享同一份抽象与用法（P1/S11）。
//
// 运行（在 examples/tool_loop_agent 目录）：
//
//	go run . --task "读取 ../data.json，用 calculator 计算 sales 总和并给出中文摘要"
//
// 启用记忆持久化与注入（跨会话）：
//
//	go run . --persona-id my-agent --memory ./mem.db --embed-dim 1536 --seed-memory "用户偏好中文摘要"
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
	task := flag.String("task", "", "任务描述；为空时读取 stdin")
	// v2.0 Persona + Memory 抽象（A1/A2/S11）。
	personaID := flag.String("persona-id", "tool-loop-demo", "Persona ID（跨会话身份锚点）")
	personaName := flag.String("persona-name", "数据分析助手", "Persona 名称")
	memoryPath := flag.String("memory", "", "记忆 SQLite 文件路径（空=不启用记忆注入）")
	embedDim := flag.Int("embed-dim", 1536, "embedding 维度（需与所选 embedding 模型一致）")
	seedFact := flag.String("seed-memory", "", "启动时写入一条 LongTerm 记忆（仅演示；真实场景由历史 Run 产出）")
	flag.Parse()

	bURL := firstNonEmpty(*baseURL, os.Getenv("MTS_BASEURL"))
	aKey := firstNonEmpty(*apiKey, os.Getenv("MTS_API_KEY"))
	mName := firstNonEmpty(*model, os.Getenv("MTS_MODEL"))
	if bURL == "" || aKey == "" || mName == "" {
		log.Fatal("需要 MTS_BASEURL / MTS_API_KEY / MTS_MODEL（或对应 flag）")
	}

	// 1. 模型（OpenAI adapter 同时实现 Model 与 EmbeddingProvider）。
	client, err := modelopenai.New(modelopenai.Config{BaseURL: bURL, APIKey: aKey, Model: mName})
	if err != nil {
		log.Fatalf("初始化模型: %v", err)
	}

	// 2. Persona + MemoryStore + ContextBuilder（v2.0 三件套，A1/A2/S11/P1）。
	//    与 research_agent / workflow_agent 用同一份构造路径。
	persona := &agentcontract.Persona{
		ID:           *personaID,
		Name:         *personaName,
		Role:         "tool-loop-analyst",
		SystemPrompt: "你是一名严谨的数据分析师，用工具读取数据、用计算器求值，并给出中文摘要。",
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
		// 演示：启动时种子一条 LongTerm 记忆（真实场景由历史 Run 产出 Evidence 写入）。
		if *seedFact != "" {
			if err := saveLongTerm(context.Background(), memStore, persona.ID, *seedFact); err != nil {
				log.Fatalf("种子记忆写入: %v", err)
			}
		}
	}

	// 3. 注册工具并组装 Agent（Options 装配三件套，agent-core 无需改源码）。
	reg := agentcore.NewRegistry()
	for _, t := range []agentcore.Tool{tools.FileReader{}, tools.Calculator{}} {
		if err := reg.Register(t); err != nil {
			log.Fatalf("注册工具 %s: %v", t.Name(), err)
		}
	}
	agent := agentcore.New(client, reg, agentcore.Options{
		ContextBuilder: ctxBuilder,
		Persona:        persona,
		MemoryStore:    memStore, // nil 时 agent-core 自动跳过记忆注入
		OnEvent: func(ev agentcore.Event) {
			switch ev.Kind {
			case agentcore.EventToolStart:
				fmt.Printf("[工具] %s\n", ev.Tool)
			case agentcore.EventMemoryInjected:
				if ev.Content != "" {
					fmt.Printf("[记忆注入] %s...\n", oneLine(ev.Content, 80))
				}
			case agentcore.EventModelDelta:
				fmt.Print(ev.Content)
			}
		},
	})

	// 4. 任务输入
	input := *task
	if input == "" {
		fmt.Print("任务：")
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			log.Fatal("无任务输入")
		}
		input = strings.TrimSpace(line)
	}

	// 5. 运行
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	res, err := agent.Run(ctx, input)
	if err != nil {
		log.Fatalf("运行失败: %v", err)
	}

	// 6. 结果
	fmt.Printf("\n=== 结果 ===\n%s\n", res.FinalMessage.Content)
	fmt.Printf("=== 统计 ===\n工具调用: %d | 模型轮次: %d | tokens: %d\n",
		res.ToolCalls, res.Iterations, res.Usage.TotalTokens)
}

// saveLongTerm 写一条 LongTerm 记忆（跨会话保留）。
func saveLongTerm(ctx context.Context, store *agentruntime.VectorMemoryStore, personaID, content string) error {
	m := &agentcontract.Memory{
		ID:        fmt.Sprintf("%s-longterm-%d", personaID, time.Now().UnixNano()),
		PersonaID: personaID,
		Layer:     agentcontract.MemoryLayerLongTerm,
		Content:   content,
		Tags:      []string{"seed"},
		CreatedAt: time.Now(),
	}
	return store.Save(ctx, m)
}

// oneLine 把字符串压成单行并截断到 maxRunes。
func oneLine(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
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

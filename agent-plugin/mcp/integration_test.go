package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	agentcore "github.com/mantis-layer/mts/agent-core"
)

// TestPythonServerInterop 跨语言真实联调：Go Client ↔ Python MCP stdio server。
// 验证跨进程协议兼容（initialize/tools/list/tools/call over stdio JSON-RPC）。
func TestPythonServerInterop(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 不可用，跳过跨语言 MCP 联调")
	}
	serverPath := filepath.Join("..", "..", "examples", "mcp", "python_echo_server.py")
	if _, err := os.Stat(serverPath); err != nil {
		t.Fatalf("Python server 不存在: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := NewClient(ctx, "python3", serverPath)
	if err != nil {
		t.Fatalf("NewClient(python3): %v", err)
	}
	defer client.Close()

	// tools/list：应列出 echo 与 sum
	tools := client.Tools()
	if len(tools) != 2 {
		t.Fatalf("工具数 = %d，期望 2: %+v", len(tools), tools)
	}
	names := map[string]ToolInfo{}
	for _, ti := range tools {
		names[ti.Name] = ti
	}
	if _, ok := names["echo"]; !ok {
		t.Fatalf("缺少 echo 工具: %+v", tools)
	}

	// tools/call：echo
	out, isErr, err := client.CallTool(ctx, "echo", map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("CallTool(echo): %v", err)
	}
	if isErr || out != "echo: hello" {
		t.Fatalf("echo 结果 = %q (isErr=%v)", out, isErr)
	}

	// tools/call：sum
	out, isErr, err = client.CallTool(ctx, "sum", map[string]any{"numbers": []float64{1, 2, 3}})
	if err != nil {
		t.Fatalf("CallTool(sum): %v", err)
	}
	if isErr || out != "6" {
		t.Fatalf("sum 结果 = %q (isErr=%v)", out, isErr)
	}

	// 非法参数容错：server 不得崩溃，返回 isError
	out, isErr, err = client.CallTool(ctx, "sum", map[string]any{"numbers": []any{"abc"}})
	if err != nil {
		t.Fatalf("CallTool(sum 非法参数): %v", err)
	}
	if !isErr || out == "" {
		t.Fatalf("非法参数应返回 isError: out=%q isErr=%v", out, isErr)
	}

	// 包装为 agentcore.Tool 并注册（端到端：可被 Agent 使用）
	reg := agentcore.NewRegistry()
	adapter := NewToolAdapter(client, names["echo"], "py_echo")
	if err := reg.Register(adapter); err != nil {
		t.Fatalf("Register(py_echo): %v", err)
	}
	res, err := adapter.Execute(ctx, map[string]any{"msg": "agent 调用"})
	if err != nil {
		t.Fatalf("Execute(py_echo): %v", err)
	}
	if res["content"] != "echo: agent 调用" {
		t.Fatalf("Execute 结果 = %+v", res)
	}
}

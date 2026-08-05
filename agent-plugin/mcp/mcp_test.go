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

// buildTestServer 编译 testdata 假 MCP server 到临时目录，返回可执行路径。
func buildTestServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mcpserver")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/mcp_server.go")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("构建假 server 失败: %v\n%s", err, out)
	}
	return bin
}

func TestMCPClient_ListAndCallTool(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("无 go 工具链")
	}
	bin := buildTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	defer client.Close()

	tools := client.Tools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("工具列表=%+v", tools)
	}

	text, isErr, err := client.CallTool(ctx, "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if isErr {
		t.Fatal("不应标记为错误")
	}
	if text != "echo: hello" {
		t.Fatalf("返回=%q 期望 echo: hello", text)
	}
}

func TestMcpToolAdapter_ImplementsCoreTool(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("无 go 工具链")
	}
	bin := buildTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	defer client.Close()

	adapter := NewToolAdapter(client, client.Tools()[0], "")
	if adapter.Name() != "echo" {
		t.Fatalf("Name=%q", adapter.Name())
	}

	// 通过 Schema 校验后再执行（模拟 agent-core 调用路径）
	if err := agentcore.ValidateJSONSchema(adapter.Parameters(), map[string]any{"text": "hi"}); err != nil {
		t.Fatalf("Schema 校验失败: %v", err)
	}
	out, err := adapter.Execute(ctx, map[string]any{"text": "hi"})
	if err != nil {
		t.Fatalf("Execute 失败: %v", err)
	}
	if out["content"] != "echo: hi" {
		t.Fatalf("输出=%v", out)
	}

	// 缺必填字段时 Schema 校验应拒绝
	if err := agentcore.ValidateJSONSchema(adapter.Parameters(), map[string]any{}); err == nil {
		t.Fatal("缺必填 text 应报错")
	}
}

func TestMCPClient_ServerExit(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("无 go 工具链")
	}
	bin := buildTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	// 立即杀掉 server
	_ = client.cmd.Process.Kill()
	_ = client.Close()

	if _, _, err := client.CallTool(ctx, "echo", map[string]any{"text": "x"}); err == nil {
		t.Fatal("server 退出后 CallTool 应报错")
	}
	_ = os.Remove(bin)
}

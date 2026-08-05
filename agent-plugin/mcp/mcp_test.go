package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// TestMCPClient_RPCError 验证 server 返回 JSON-RPC error 时被传播为 Go error。
func TestMCPClient_RPCError(t *testing.T) {
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

	_, _, err = client.CallTool(ctx, "echo", map[string]any{"text": "boom"})
	if err == nil {
		t.Fatal("RPC error 应被传播")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("错误信息=%v", err)
	}
}

// TestMCPClient_Concurrent 验证并发 CallTool 无数据竞争（配合 -race）。
func TestMCPClient_Concurrent(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("无 go 工具链")
	}
	bin := buildTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := NewClient(ctx, bin)
	if err != nil {
		t.Fatalf("NewClient 失败: %v", err)
	}
	defer client.Close()

	const n = 10
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			text := fmt.Sprintf("msg-%d", i)
			got, _, err := client.CallTool(ctx, "echo", map[string]any{"text": text})
			if err != nil {
				errs <- err
				return
			}
			if got != "echo: "+text {
				errs <- fmt.Errorf("响应错配: got=%q want=%q", got, "echo: "+text)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("并发错误: %v", err)
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

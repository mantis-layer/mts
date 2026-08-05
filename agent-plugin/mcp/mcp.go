// Package mcp 提供 MCP（Model Context Protocol）Tool Adapter：
// 通过 stdio 上的 JSON-RPC 2.0 与 MCP server 通信，
// 将其 tools/list 的工具暴露为 agent-core Tool（FR-006 MCP Tool Adapter）。
//
// v0.1 为最小实现：initialize → initialized → tools/list → tools/call，
// 无第三方依赖；工具参数按 JSON Schema 透传。
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProtocolVersion 是本客户端声明的 MCP 协议版本。
const ProtocolVersion = "2024-11-05"

// Client 是 MCP stdio 客户端。
type Client struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	mu      sync.Mutex
	nextID  int
	pending map[int]chan json.RawMessage
	done    chan struct{}

	// 工具缓存（initialize 时从 tools/list 拉取）
	tools []ToolInfo
}

// ToolInfo 描述 MCP server 暴露的一个工具。
type ToolInfo struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// NewClient 启动 MCP server 子进程并完成 initialize 握手。
func NewClient(ctx context.Context, command string, args ...string) (*Client, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin 管道: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout 管道: %w", err)
	}
	cmd.Stderr = nil // 保持静默；如需调试可接管

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: 启动 server %q: %w", command, err)
	}

	c := &Client{
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		pending: make(map[int]chan json.RawMessage),
		done:    make(chan struct{}),
	}
	go c.readLoop()

	initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := c.initialize(initCtx); err != nil {
		_ = c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) initialize(ctx context.Context) error {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "mts-agent", "version": "0.1.0"},
	}, &result); err != nil {
		return fmt.Errorf("mcp: initialize: %w", err)
	}
	if err := c.notify("notifications/initialized", nil); err != nil {
		return fmt.Errorf("mcp: initialized 通知: %w", err)
	}
	var list struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", nil, &list); err != nil {
		return fmt.Errorf("mcp: tools/list: %w", err)
	}
	c.tools = list.Tools
	return nil
}

// Tools 返回 server 暴露的工具列表。
func (c *Client) Tools() []ToolInfo {
	return c.tools
}

// CallTool 调用 server 的一个工具。
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (string, bool, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return "", false, err
	}
	var sb strings.Builder
	for _, ch := range result.Content {
		if ch.Type == "text" {
			sb.WriteString(ch.Text)
		}
	}
	return sb.String(), result.IsError, nil
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	id := c.nextID + 1
	c.nextID = id
	respCh := make(chan json.RawMessage, 1)
	c.mu.Lock()
	c.pending[id] = respCh
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("mcp: 序列化请求: %w", err)
	}
	c.mu.Lock()
	_, err = fmt.Fprintln(c.stdin, string(payload))
	c.mu.Unlock()
	if err != nil {
		return fmt.Errorf("mcp: 写入请求: %w", err)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("mcp: %s 等待响应超时: %w", method, ctx.Err())
	case <-c.done:
		return fmt.Errorf("mcp: server 已退出，未收到 %s 响应", method)
	case raw, ok := <-respCh:
		if !ok {
			return fmt.Errorf("mcp: server 已退出，未收到 %s 响应", method)
		}
		return json.Unmarshal(raw, result)
	}
}

func (c *Client) notify(method string, params any) error {
	req := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err = fmt.Fprintln(c.stdin, string(payload))
	return err
}

// readLoop 读取 server 输出，将响应路由到对应 pending channel。
func (c *Client) readLoop() {
	defer close(c.done)
	for c.scanner.Scan() {
		line := c.scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var resp rpcResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			continue // 忽略无法解析的行（含 server 日志）
		}
		if resp.ID == 0 {
			continue // 服务端主动通知，v0.1 忽略
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		c.mu.Unlock()
		if !ok {
			continue
		}
		if resp.Error != nil {
			ch <- mustMarshal(map[string]any{"error": resp.Error.Message})
			continue
		}
		ch <- resp.Result
	}
	// server 退出：唤醒所有 pending
	c.mu.Lock()
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = map[int]chan json.RawMessage{}
	c.mu.Unlock()
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Close 关闭 stdin 并等待子进程退出。
func (c *Client) Close() error {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	waitCh := make(chan error, 1)
	go func() { waitCh <- c.cmd.Wait() }()
	select {
	case <-waitCh:
	case <-time.After(3 * time.Second):
		_ = c.cmd.Process.Kill()
		<-waitCh
	}
	return nil
}

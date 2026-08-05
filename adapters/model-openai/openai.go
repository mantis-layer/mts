// Package modelopenai 实现 OpenAI 兼容协议（含中转站/聚合服务）的模型 Adapter。
//
// 配置只需 baseurl + apiKey + model 三要素；任何提供 OpenAI 兼容
// /chat/completions 接口的端点（含中转站）均可接入。
package modelopenai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// Config 是 OpenAI 兼容 Adapter 的配置。
type Config struct {
	// BaseURL 兼容端点根地址，如 https://api.aicodewith.com。
	BaseURL string
	// APIKey 认证密钥。
	APIKey string
	// Model 模型名称。
	Model string
	// HTTPClient 可选；默认使用 60s 超时的客户端。
	HTTPClient *http.Client
}

// Client 实现 agentmodel.Model，通过 OpenAI 兼容 /chat/completions 接口通信。
type Client struct {
	cfg  Config
	http *http.Client
}

// New 校验配置并构造 Client。
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("modelopenai: BaseURL 不能为空")
	}
	if cfg.APIKey == "" {
		return nil, errors.New("modelopenai: APIKey 不能为空")
	}
	if cfg.Model == "" {
		return nil, errors.New("modelopenai: Model 不能为空")
	}
	hc := cfg.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{cfg: cfg, http: hc}, nil
}

// ModelName 返回配置的模型名称。
func (c *Client) ModelName() string { return c.cfg.Model }

func (c *Client) endpoint() string {
	return strings.TrimRight(c.cfg.BaseURL, "/") + "/chat/completions"
}

type chatRequest struct {
	Model    string                  `json:"model"`
	Messages []agentmodel.Message    `json:"messages"`
	Tools    []agentmodel.ToolSchema `json:"tools,omitempty"`
	Stream   bool                    `json:"stream"`
}

// Complete 执行一次非流式补全。
func (c *Client) Complete(ctx context.Context, req agentmodel.Request) (agentmodel.Response, error) {
	payload := chatRequest{
		Model:    c.cfg.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
		Stream:   false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return agentmodel.Response{}, fmt.Errorf("modelopenai: 序列化请求: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return agentmodel.Response{}, fmt.Errorf("modelopenai: 构造请求: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return agentmodel.Response{}, mapTransportError(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return agentmodel.Response{}, mapHTTPError(resp.StatusCode, string(raw))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return agentmodel.Response{}, &agentmodel.ModelError{Kind: agentmodel.ErrorKindUnknown, Message: "解析响应失败", Cause: err}
	}
	if len(out.Choices) == 0 {
		return agentmodel.Response{}, &agentmodel.ModelError{Kind: agentmodel.ErrorKindServer, Message: "响应缺少 choices"}
	}

	msg := agentmodel.Message{Role: agentmodel.RoleAssistant, Content: out.Choices[0].Message.Content}
	for _, tc := range out.Choices[0].Message.ToolCalls {
		msg.ToolCalls = append(msg.ToolCalls, agentmodel.ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return agentmodel.Response{
		Message: msg,
		Usage: agentmodel.Usage{
			PromptTokens:     out.Usage.PromptTokens,
			CompletionTokens: out.Usage.CompletionTokens,
			TotalTokens:      out.Usage.TotalTokens,
		},
		FinishReason: agentmodel.FinishReason(out.Choices[0].FinishReason),
	}, nil
}

// Stream 执行一次流式补全，逐段返回增量事件。
func (c *Client) Stream(ctx context.Context, req agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	payload := chatRequest{
		Model:    c.cfg.Model,
		Messages: req.Messages,
		Tools:    req.Tools,
		Stream:   true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("modelopenai: 序列化请求: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("modelopenai: 构造请求: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, mapTransportError(err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, mapHTTPError(resp.StatusCode, string(raw))
	}

	ch := make(chan agentmodel.StreamEvent, 64)
	go c.streamLoop(ctx, resp, ch)
	return ch, nil
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (c *Client) streamLoop(ctx context.Context, resp *http.Response, ch chan<- agentmodel.StreamEvent) {
	defer close(ch)
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	// 按 index 累积流式 tool call 分片（id/name/arguments 逐片拼接），
	// 在流结束（finish_reason 或连接关闭）时统一发出完整 ToolCall。
	// 不能在首个分片就发出——此时 arguments 通常尚未开始。
	toolAcc := map[int]*agentmodel.ToolCall{}
	finished := false

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventError, Error: fmt.Errorf("modelopenai: 解析流式分片: %w", err)}
			return
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		choice := chunk.Choices[0]

		if choice.Delta.Content != "" {
			ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventDelta, Delta: choice.Delta.Content}
		}
		for _, tc := range choice.Delta.ToolCalls {
			acc, ok := toolAcc[tc.Index]
			if !ok {
				acc = &agentmodel.ToolCall{ID: tc.ID, Name: tc.Function.Name}
				toolAcc[tc.Index] = acc
			}
			if tc.ID != "" {
				acc.ID = tc.ID
			}
			if tc.Function.Name != "" {
				acc.Name = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				acc.Arguments += tc.Function.Arguments
			}
		}
		if chunk.Usage != nil {
			u := agentmodel.Usage{
				PromptTokens:     chunk.Usage.PromptTokens,
				CompletionTokens: chunk.Usage.CompletionTokens,
				TotalTokens:      chunk.Usage.TotalTokens,
			}
			ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventUsage, Usage: &u}
		}
		if choice.FinishReason != "" {
			emitAccumulatedToolCalls(ch, toolAcc)
			finished = true
			ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReason(choice.FinishReason)}
		}
	}
	// 防御：流结束但未收到 finish_reason（异常截断），仍有已累积工具调用。
	if !finished && len(toolAcc) > 0 {
		emitAccumulatedToolCalls(ch, toolAcc)
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventError, Error: fmt.Errorf("modelopenai: 读取流失败: %w", err)}
	}
}

// emitAccumulatedToolCalls 按 index 顺序发出全部已累积的完整 ToolCall。
func emitAccumulatedToolCalls(ch chan<- agentmodel.StreamEvent, toolAcc map[int]*agentmodel.ToolCall) {
	indices := make([]int, 0, len(toolAcc))
	for idx := range toolAcc {
		indices = append(indices, idx)
	}
	sort.Ints(indices)
	for _, idx := range indices {
		acc := toolAcc[idx]
		if acc.ID == "" || acc.Name == "" {
			continue // 不完整（异常流）则跳过
		}
		clone := *acc
		ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventToolCall, ToolCall: &clone}
	}
}

// mapTransportError 将网络/超时错误映射为结构化 ModelError。
func mapTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindTimeout, Message: "请求超时", Cause: err}
	}
	return &agentmodel.ModelError{Kind: agentmodel.ErrorKindNetwork, Message: "网络错误", Cause: err}
}

// mapHTTPError 将 HTTP 状态码映射为结构化 ModelError。
func mapHTTPError(status int, body string) error {
	msg := body
	if len(msg) > 300 {
		msg = msg[:300]
	}
	switch {
	case status == http.StatusBadRequest:
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindInvalidRequest, Message: msg}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindAuthentication, Message: msg}
	case status == http.StatusTooManyRequests:
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindRateLimit, Message: msg, Retryable: true}
	case status >= 500:
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindServer, Message: msg, Retryable: true}
	default:
		return &agentmodel.ModelError{Kind: agentmodel.ErrorKindUnknown, Message: msg}
	}
}

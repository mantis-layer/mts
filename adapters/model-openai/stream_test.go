package modelopenai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// sseChunk 构造一个 OpenAI 兼容的 SSE data 行。
func sseChunk(payload string) string {
	return "data: " + payload + "\n\n"
}

// TestStream_ToolCallArgumentsAssembled 验证流式 tool_call 分片被正确拼接
// （回归：修复前首个分片即发出事件，arguments 恒为空导致工具循环失败）。
func TestStream_ToolCallArgumentsAssembled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 标准 OpenAI 分片顺序：先 id+name，再 arguments 分片，最后 finish_reason。
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"calculator","arguments":""}}]}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"expression\":\"1+2\"}"}}]}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":""}}]},"finish_reason":"tool_calls"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	ch, err := c.Stream(context.Background(), agentmodel.Request{Messages: userMsg("计算")})
	if err != nil {
		t.Fatalf("Stream 启动失败: %v", err)
	}
	var tc *agentmodel.ToolCall
	for ev := range ch {
		if ev.Kind == agentmodel.StreamEventToolCall && ev.ToolCall != nil {
			tc = ev.ToolCall
		}
		if ev.Kind == agentmodel.StreamEventError && ev.Error != nil {
			t.Fatalf("流中错误: %v", ev.Error)
		}
	}
	if tc == nil {
		t.Fatal("未收到 ToolCall 事件")
	}
	if tc.Name != "calculator" || tc.ID != "call_1" {
		t.Fatalf("tool_call=%+v", tc)
	}
	if tc.Arguments != `{"expression":"1+2"}` {
		t.Fatalf("Arguments 未完整拼接: %q", tc.Arguments)
	}
}

// TestStream_MultipleToolCallsOrdered 验证多个 tool call 按 index 顺序发出。
func TestStream_MultipleToolCallsOrdered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c0","function":{"name":"a"}},{"index":1,"id":"c1","function":{"name":"b","arguments":"{\"x\":1}"}}]}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"y\":2}"}}]}}],"finish_reason":"tool_calls"}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	ch, err := c.Stream(context.Background(), agentmodel.Request{Messages: userMsg("多工具")})
	if err != nil {
		t.Fatalf("Stream 启动失败: %v", err)
	}
	var calls []agentmodel.ToolCall
	for ev := range ch {
		if ev.Kind == agentmodel.StreamEventToolCall && ev.ToolCall != nil {
			calls = append(calls, *ev.ToolCall)
		}
		if ev.Kind == agentmodel.StreamEventError && ev.Error != nil {
			t.Fatalf("流中错误: %v", ev.Error)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("期望 2 个 tool call，实际 %d: %+v", len(calls), calls)
	}
	if calls[0].Name != "a" || calls[0].Arguments != `{"y":2}` {
		t.Fatalf("calls[0]=%+v", calls[0])
	}
	if calls[1].Name != "b" || calls[1].Arguments != `{"x":1}` {
		t.Fatalf("calls[1]=%+v", calls[1])
	}
}

// TestStream_TextDeltaAndFinish 验证纯文本流的 delta 与 finish 事件。
func TestStream_TextDeltaAndFinish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"你"}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{"content":"好"}}]}`))
		fmt.Fprint(w, sseChunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	c, err := New(Config{BaseURL: server.URL, APIKey: "test-key", Model: "test-model"})
	if err != nil {
		t.Fatalf("构造 client 失败: %v", err)
	}
	ch, err := c.Stream(context.Background(), agentmodel.Request{Messages: userMsg("hi")})
	if err != nil {
		t.Fatalf("Stream 启动失败: %v", err)
	}
	var got strings.Builder
	sawFinish := false
	for ev := range ch {
		switch ev.Kind {
		case agentmodel.StreamEventDelta:
			got.WriteString(ev.Delta)
		case agentmodel.StreamEventFinish:
			sawFinish = true
		case agentmodel.StreamEventError:
			if ev.Error != nil {
				t.Fatalf("流中错误: %v", ev.Error)
			}
		}
	}
	if got.String() != "你好" {
		t.Fatalf("delta=%q 期望 你好", got.String())
	}
	if !sawFinish {
		t.Fatal("未收到 finish 事件")
	}
}

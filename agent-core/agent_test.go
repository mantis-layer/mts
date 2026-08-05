package agentcore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// mockModel 按预置的流序列返回事件，用于离线测试 Agent Loop。
type mockModel struct {
	mu        sync.Mutex
	streams   [][]agentmodel.StreamEvent
	streamErr error
	calls     int
}

func (m *mockModel) ModelName() string { return "mock" }

func (m *mockModel) Complete(_ context.Context, req agentmodel.Request) (agentmodel.Response, error) {
	assistant := agentmodel.Message{Role: agentmodel.RoleAssistant}
	var usage agentmodel.Usage
	for _, ev := range m.nextStream() {
		switch ev.Kind {
		case agentmodel.StreamEventDelta:
			assistant.Content += ev.Delta
		case agentmodel.StreamEventToolCall:
			if ev.ToolCall != nil {
				assistant.ToolCalls = append(assistant.ToolCalls, *ev.ToolCall)
			}
		case agentmodel.StreamEventUsage:
			if ev.Usage != nil {
				usage = *ev.Usage
			}
		}
	}
	return agentmodel.Response{Message: assistant, Usage: usage}, nil
}

func (m *mockModel) Stream(_ context.Context, _ agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	evs := m.nextStream()
	ch := make(chan agentmodel.StreamEvent, len(evs))
	for _, e := range evs {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func (m *mockModel) nextStream() []agentmodel.StreamEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.streamErr != nil {
		return nil
	}
	if m.calls > len(m.streams) {
		return nil
	}
	return m.streams[m.calls-1]
}

func textStream(text string) []agentmodel.StreamEvent {
	return []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventDelta, Delta: text},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop},
	}
}

func toolStream(name, args string) []agentmodel.StreamEvent {
	return []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "call_1", Name: name, Arguments: args}},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
	}
}

// mockTool 记录调用并可配置结果/错误/阻塞行为。
type mockTool struct {
	name   string
	schema map[string]any
	fn     func(ctx context.Context, input map[string]any) (map[string]any, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return "mock tool " + t.name }
func (t *mockTool) Parameters() map[string]any {
	if t.schema != nil {
		return t.schema
	}
	return map[string]any{"type": "object"}
}
func (t *mockTool) Execute(ctx context.Context, input map[string]any) (map[string]any, error) {
	if t.fn != nil {
		return t.fn(ctx, input)
	}
	return map[string]any{"ok": true}, nil
}

func newTestAgent(m *mockModel, reg *Registry, opts Options) (*Agent, *[]Event) {
	var events []Event
	opts.OnEvent = func(ev Event) { events = append(events, ev) }
	if opts.MaxToolCalls == 0 {
		opts.MaxToolCalls = 5
	}
	if opts.MaxIterations == 0 {
		opts.MaxIterations = 5
	}
	return New(m, reg, opts), &events
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestRun_ToolLoopCompletes(t *testing.T) {
	reg := NewRegistry()
	var gotInput map[string]any
	if err := reg.Register(&mockTool{
		name:   "calculator",
		schema: map[string]any{"type": "object", "properties": map[string]any{"expression": map[string]any{"type": "string"}}, "required": []string{"expression"}},
		fn: func(_ context.Context, input map[string]any) (map[string]any, error) {
			gotInput = input
			return map[string]any{"result": 3.0}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		toolStream("calculator", mustJSON(t, map[string]any{"expression": "1+2"})),
		textStream("结果是 3"),
	}}
	a, _ := newTestAgent(m, reg, Options{})
	res, err := a.Run(context.Background(), "计算 1+2")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.FinalMessage.Content != "结果是 3" {
		t.Fatalf("最终消息=%q", res.FinalMessage.Content)
	}
	if res.ToolCalls != 1 {
		t.Fatalf("ToolCalls=%d 期望 1", res.ToolCalls)
	}
	if res.Iterations != 2 {
		t.Fatalf("Iterations=%d 期望 2", res.Iterations)
	}
	if gotInput["expression"] != "1+2" {
		t.Fatalf("工具收到参数=%v", gotInput)
	}
}

func TestRun_TwoToolCallsInOneTurn(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "calc_a"}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(&mockTool{name: "calc_b"}); err != nil {
		t.Fatal(err)
	}
	multi := []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "c1", Name: "calc_a", Arguments: "{}"}},
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "c2", Name: "calc_b", Arguments: "{}"}},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{multi, textStream("done")}}
	a, events := newTestAgent(m, reg, Options{})
	res, err := a.Run(context.Background(), "两个工具")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.ToolCalls != 2 {
		t.Fatalf("ToolCalls=%d 期望 2", res.ToolCalls)
	}
	toolStarts := 0
	for _, ev := range *events {
		if ev.Kind == EventToolStart {
			toolStarts++
		}
	}
	if toolStarts != 2 {
		t.Fatalf("tool.start 事件数=%d 期望 2", toolStarts)
	}
}

func TestRun_ToolNotFound(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		toolStream("missing_tool", "{}"),
		textStream("工具不可用"),
	}}
	a, events := newTestAgent(m, reg, Options{})
	res, err := a.Run(context.Background(), "调用缺失工具")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.FinalMessage.Content != "工具不可用" {
		t.Fatalf("最终消息=%q", res.FinalMessage.Content)
	}
	for _, ev := range *events {
		if ev.Kind == EventToolError {
			var te *ToolError
			if !errors.As(ev.Error, &te) || te.Code != "tool_not_found" {
				t.Fatalf("期望 tool_not_found 结构化错误，实际 %v", ev.Error)
			}
			return
		}
	}
	t.Fatal("未收到 tool_not_found 错误事件")
}

func TestRun_SchemaValidationError(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&mockTool{
		name:   "strict",
		schema: map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "number"}}, "required": []string{"x"}},
	}); err != nil {
		t.Fatal(err)
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		toolStream("strict", `{}`), // 缺必填 x
		textStream("参数不合法"),
	}}
	a, events := newTestAgent(m, reg, Options{})
	if _, err := a.Run(context.Background(), "非法参数"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	for _, ev := range *events {
		if ev.Kind == EventToolError {
			var te *ToolError
			if !errors.As(ev.Error, &te) || te.Code != "schema_validation" {
				t.Fatalf("期望 schema_validation 错误，实际 %v", ev.Error)
			}
			return
		}
	}
	t.Fatal("未收到 schema_validation 错误事件")
}

func TestRun_Cancellation(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{} // 空流：Stream 立即返回，但用取消的 ctx
	a, _ := newTestAgent(m, reg, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消
	res, err := a.Run(ctx, "取消测试")
	if err == nil {
		t.Fatal("期望取消错误")
	}
	if !res.Aborted {
		t.Fatal("期望 Aborted=true")
	}
}

func TestRun_ToolBudgetExceeded(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "t1"}); err != nil {
		t.Fatal(err)
	}
	multi := []agentmodel.StreamEvent{
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "c1", Name: "t1", Arguments: "{}"}},
		{Kind: agentmodel.StreamEventToolCall, ToolCall: &agentmodel.ToolCall{ID: "c2", Name: "t1", Arguments: "{}"}},
		{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonToolCalls},
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{multi}}
	a, _ := newTestAgent(m, reg, Options{MaxToolCalls: 1})
	_, err := a.Run(context.Background(), "超预算")
	if err == nil {
		t.Fatal("期望预算耗尽错误")
	}
	if !strings.Contains(err.Error(), "预算耗尽") {
		t.Fatalf("错误=%v", err)
	}
}

func TestRun_ToolTimeout(t *testing.T) {
	reg := NewRegistry()
	cancelled := make(chan struct{}, 1)
	if err := reg.Register(&mockTool{
		name: "slow",
		fn: func(ctx context.Context, _ map[string]any) (map[string]any, error) {
			<-ctx.Done() // 阻塞直到超时取消
			cancelled <- struct{}{}
			return nil, ctx.Err()
		},
	}); err != nil {
		t.Fatal(err)
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		toolStream("slow", "{}"),
		textStream("超时已处理"),
	}}
	a, events := newTestAgent(m, reg, Options{ToolTimeout: 100 * time.Millisecond})
	res, err := a.Run(context.Background(), "超时工具")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.FinalMessage.Content != "超时已处理" {
		t.Fatalf("最终消息=%q", res.FinalMessage.Content)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("工具未被取消")
	}
	for _, ev := range *events {
		if ev.Kind == EventToolError {
			return // 工具错误已结构化回模型
		}
	}
	t.Fatal("未收到工具错误事件")
}

func TestRun_ContextHook(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	hooked := false
	a, _ := newTestAgent(m, reg, Options{
		ContextHook: func(_ context.Context, msgs []agentmodel.Message) []agentmodel.Message {
			hooked = true
			out := append([]agentmodel.Message(nil), msgs...)
			out = append(out, agentmodel.Message{Role: agentmodel.RoleSystem, Content: "附加指令"})
			return out
		},
	})
	if _, err := a.Run(context.Background(), "hook 测试"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if !hooked {
		t.Fatal("ContextHook 未被调用")
	}
}

func TestRun_SteeringAborts(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("不应到达")}}
	a, _ := newTestAgent(m, reg, Options{
		Steering: func(_ context.Context, _ []agentmodel.Message) ([]agentmodel.Message, error) {
			return nil, fmt.Errorf("steering 中止")
		},
	})
	if _, err := a.Run(context.Background(), "steering"); err == nil || !strings.Contains(err.Error(), "steering 中止") {
		t.Fatalf("期望 steering 中止错误，实际 %v", err)
	}
}

func TestRun_UsageAccumulated(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		append(toolStream("t", "{}"), agentmodel.StreamEvent{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}}),
		append(textStream("fin"), agentmodel.StreamEvent{Kind: agentmodel.StreamEventUsage, Usage: &agentmodel.Usage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}}),
	}}
	if err := reg.Register(&mockTool{name: "t"}); err != nil {
		t.Fatal(err)
	}
	a, _ := newTestAgent(m, reg, Options{})
	res, err := a.Run(context.Background(), "usage")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.Usage.TotalTokens != 45 || res.Usage.PromptTokens != 30 {
		t.Fatalf("Usage 未累计: %+v", res.Usage)
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}

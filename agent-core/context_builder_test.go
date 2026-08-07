package agentcore

import (
	"context"
	"errors"
	"strings"
	"testing"

	agentcontract "github.com/mantis-layer/mts/agent-contract"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// recordingBuilder 记录调用次数/顺序与收到的当前消息，返回可配置结果。
type recordingBuilder struct {
	calls    int
	onBuild  func()
	msg      agentmodel.Message
	err      error
	lastMsg  agentmodel.Message
	lastCall int
}

func (b *recordingBuilder) Build(_ context.Context, _ agentcontract.Persona, _ agentcontract.MemoryStore, msg agentmodel.Message) (agentmodel.Message, error) {
	b.calls++
	b.lastMsg = msg
	b.lastCall = b.calls
	if b.onBuild != nil {
		b.onBuild()
	}
	return b.msg, b.err
}

// fakeMemStore 实现 MemoryStore，用于 agent-core 挂载测试。
type fakeMemStore struct{}

func (fakeMemStore) Save(context.Context, *agentcontract.Memory) error { return nil }
func (fakeMemStore) Query(context.Context, string, agentcontract.MemoryLayer, agentcontract.QueryOptions) ([]agentcontract.Memory, error) {
	return nil, nil
}
func (fakeMemStore) Delete(context.Context, string) error { return nil }

var _ agentcontract.MemoryStore = fakeMemStore{}

// capturingModel 包装 mockModel，记录每次模型调用收到的消息视图。
type capturingModel struct {
	inner agentmodel.Model
	views [][]agentmodel.Message
}

func (c *capturingModel) ModelName() string {
	if named, ok := c.inner.(interface{ ModelName() string }); ok {
		return named.ModelName()
	}
	return "capture"
}

func (c *capturingModel) Complete(ctx context.Context, req agentmodel.Request) (agentmodel.Response, error) {
	c.views = append(c.views, req.Messages)
	return c.inner.Complete(ctx, req)
}

func (c *capturingModel) Stream(ctx context.Context, req agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	c.views = append(c.views, req.Messages)
	return c.inner.Stream(ctx, req)
}

var _ agentmodel.Model = (*capturingModel)(nil)

const testMemoryContent = "记忆：用户偏好简洁回答"

func newMemOptions(builder agentcontract.ContextBuilder, withPersona bool, withStore bool) Options {
	opts := Options{ContextBuilder: builder}
	if withPersona {
		opts.Persona = &agentcontract.Persona{ID: "persona-1", Name: "测试伙伴"}
	}
	if withStore {
		opts.MemoryStore = fakeMemStore{}
	}
	return opts
}

// TestRun_ContextBuilderPhaseOrder 覆盖 A1：ContextBuilder 在 Steering 之后、
// ContextHook 之前、模型调用之前执行。
func TestRun_ContextBuilderPhaseOrder(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	var order []string
	cb := &recordingBuilder{
		msg: agentmodel.Message{Role: agentmodel.RoleSystem, Content: testMemoryContent},
		onBuild: func() {
			order = append(order, "builder")
		},
	}
	opts := newMemOptions(cb, true, true)
	opts.Steering = func(context.Context, []agentmodel.Message) ([]agentmodel.Message, error) {
		order = append(order, "steering")
		return nil, nil
	}
	opts.ContextHook = func(context.Context, []agentmodel.Message) []agentmodel.Message {
		order = append(order, "hook")
		return nil
	}
	opts.OnEvent = func(ev Event) {
		if ev.Kind == EventModelStart {
			order = append(order, "model")
		}
	}
	a := New(m, reg, opts)
	if _, err := a.Run(context.Background(), "阶段测试"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	want := []string{"steering", "builder", "hook", "model"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("调用顺序=%v 期望 %v", order, want)
	}
}

// TestRun_DoubleHooksCoexist 覆盖 A5：ContextBuilder 与 ContextHook 共存，
// Hook 能观察到 Builder 注入的视图。
func TestRun_DoubleHooksCoexist(t *testing.T) {
	reg := NewRegistry()
	inner := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	m := &capturingModel{inner: inner}
	cb := &recordingBuilder{msg: agentmodel.Message{Role: agentmodel.RoleSystem, Content: testMemoryContent}}
	var hookSeen []agentmodel.Message
	opts := newMemOptions(cb, true, true)
	opts.ContextHook = func(_ context.Context, msgs []agentmodel.Message) []agentmodel.Message {
		hookSeen = append([]agentmodel.Message(nil), msgs...)
		return msgs
	}
	a := New(m, reg, opts)
	if _, err := a.Run(context.Background(), "双钩子"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if cb.calls != 1 {
		t.Fatalf("ContextBuilder 调用次数=%d 期望 1", cb.calls)
	}
	if len(hookSeen) == 0 || hookSeen[0].Content != testMemoryContent {
		t.Fatalf("ContextHook 应看到 Builder 注入的消息，实际 %+v", hookSeen)
	}
	if len(m.views) == 0 || len(m.views[0]) == 0 || m.views[0][0].Content != testMemoryContent {
		t.Fatalf("模型调用应收到注入后的视图，实际 %+v", m.views)
	}
}

// TestRun_MemoryInjectedEvent 覆盖 A3：每次模型调用注入记忆都发出 MemoryInjected 事件。
func TestRun_MemoryInjectedEvent(t *testing.T) {
	reg := NewRegistry()
	if err := reg.Register(&mockTool{name: "t1"}); err != nil {
		t.Fatal(err)
	}
	m := &mockModel{streams: [][]agentmodel.StreamEvent{
		toolStream("t1", "{}"),
		textStream("fin"),
	}}
	cb := &recordingBuilder{msg: agentmodel.Message{Role: agentmodel.RoleSystem, Content: testMemoryContent}}
	opts := newMemOptions(cb, true, true)
	a, events := newTestAgent(m, reg, opts)
	res, err := a.Run(context.Background(), "事件测试")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.Iterations != 2 {
		t.Fatalf("Iterations=%d 期望 2", res.Iterations)
	}
	if cb.calls != 2 {
		t.Fatalf("ContextBuilder 调用次数=%d 期望 2（每次模型调用一次）", cb.calls)
	}
	injected := 0
	for _, ev := range *events {
		if ev.Kind == EventMemoryInjected {
			injected++
			if ev.Content != testMemoryContent {
				t.Fatalf("MemoryInjected 事件 Content=%q", ev.Content)
			}
			if ev.Error != nil {
				t.Fatalf("MemoryInjected 不应带 Error: %v", ev.Error)
			}
		}
	}
	if injected != 2 {
		t.Fatalf("MemoryInjected 事件数=%d 期望 2（每次模型调用一次）", injected)
	}
}

// TestRun_NilContextBuilderNoRegression 覆盖 A4：ContextBuilder 为 nil 时
// 行为与 v0.1 等价（无 MemoryInjected 事件、ContextHook 照常工作）。
func TestRun_NilContextBuilderNoRegression(t *testing.T) {
	reg := NewRegistry()
	inner := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	m := &capturingModel{inner: inner}
	hooked := false
	opts := newMemOptions(nil, true, true)
	opts.ContextHook = func(_ context.Context, msgs []agentmodel.Message) []agentmodel.Message {
		hooked = true
		return msgs
	}
	a, events := newTestAgent(m, reg, opts)
	res, err := a.Run(context.Background(), "nil 兼容")
	if err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	if res.FinalMessage.Content != "ok" {
		t.Fatalf("最终消息=%q", res.FinalMessage.Content)
	}
	if !hooked {
		t.Fatal("ContextHook 应照常工作")
	}
	for _, ev := range *events {
		if ev.Kind == EventMemoryInjected {
			t.Fatal("ContextBuilder 为 nil 时不应发出 MemoryInjected")
		}
	}
	if len(m.views) != 1 || len(m.views[0]) != 1 {
		t.Fatalf("视图应与 v0.1 等价（无注入消息），实际 %+v", m.views)
	}
}

// TestRun_ContextBuilderErrorDegrades 覆盖边缘：检索失败降级为不注入，Run 不 fail。
func TestRun_ContextBuilderErrorDegrades(t *testing.T) {
	reg := NewRegistry()
	inner := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	m := &capturingModel{inner: inner}
	cb := &recordingBuilder{err: errors.New("检索超时")}
	opts := newMemOptions(cb, true, true)
	a, events := newTestAgent(m, reg, opts)
	if _, err := a.Run(context.Background(), "降级测试"); err != nil {
		t.Fatalf("检索失败不应使 Run 失败: %v", err)
	}
	found := false
	for _, ev := range *events {
		if ev.Kind == EventMemoryInjected {
			found = true
			if ev.Error == nil || !strings.Contains(ev.Error.Error(), "检索超时") {
				t.Fatalf("降级事件应携带错误，实际 %v", ev.Error)
			}
		}
	}
	if !found {
		t.Fatal("应发出带 Error 的 MemoryInjected 事件")
	}
	if len(m.views) != 1 || len(m.views[0]) != 1 {
		t.Fatalf("失败时不应注入消息，实际 %+v", m.views)
	}
}

// TestRun_ContextBuilderEmptyMessageSkipped 覆盖边缘：Builder 返回空消息 → 无注入、无事件。
func TestRun_ContextBuilderEmptyMessageSkipped(t *testing.T) {
	reg := NewRegistry()
	m := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
	cb := &recordingBuilder{}
	opts := newMemOptions(cb, true, true)
	a, events := newTestAgent(m, reg, opts)
	if _, err := a.Run(context.Background(), "空注入"); err != nil {
		t.Fatalf("Run 失败: %v", err)
	}
	for _, ev := range *events {
		if ev.Kind == EventMemoryInjected {
			t.Fatal("空消息不应发出 MemoryInjected")
		}
	}
}

// TestRun_ContextBuilderRequiresPersonaAndStore 覆盖边缘：
// Persona 或 MemoryStore 未配置 → 跳过记忆注入。
func TestRun_ContextBuilderRequiresPersonaAndStore(t *testing.T) {
	reg := NewRegistry()
	cb := &recordingBuilder{msg: agentmodel.Message{Role: agentmodel.RoleSystem, Content: testMemoryContent}}
	for name, opts := range map[string]Options{
		"无 Persona":     newMemOptions(cb, false, true),
		"无 MemoryStore": newMemOptions(cb, true, false),
		"两者皆无":          newMemOptions(cb, false, false),
	} {
		m := &mockModel{streams: [][]agentmodel.StreamEvent{textStream("ok")}}
		a, events := newTestAgent(m, reg, opts)
		if _, err := a.Run(context.Background(), name); err != nil {
			t.Fatalf("%s: Run 失败: %v", name, err)
		}
		for _, ev := range *events {
			if ev.Kind == EventMemoryInjected {
				t.Fatalf("%s: 未配置时应跳过注入，实际发出 MemoryInjected", name)
			}
		}
	}
	if cb.calls != 0 {
		t.Fatalf("未配置 Persona/Store 时 ContextBuilder 不应被调用，实际 %d 次", cb.calls)
	}
}

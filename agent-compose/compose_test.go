package agentcompose

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
	agentplugin "github.com/mantis-layer/mts/agent-plugin"
)

// ---- 测试工具 ----

type testTool struct{ name string }

func (t *testTool) Name() string        { return t.name }
func (t *testTool) Description() string { return "test tool " + t.name }
func (t *testTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *testTool) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

type testModelPlugin struct{ m agentmodel.Model }

func (p *testModelPlugin) Manifest() agentplugin.PluginManifest {
	return agentplugin.PluginManifest{Name: "model-a", Version: "1.0.0", APIVersion: agentplugin.CurrentAPIVersion, Type: agentplugin.PluginTypeModelProvider}
}
func (p *testModelPlugin) Init(context.Context) error { return nil }
func (p *testModelPlugin) Close() error               { return nil }
func (p *testModelPlugin) Model() (agentmodel.Model, error) {
	return p.m, nil
}

type testModelPluginB struct{ m agentmodel.Model }

func (p *testModelPluginB) Manifest() agentplugin.PluginManifest {
	return agentplugin.PluginManifest{Name: "model-b", Version: "1.0.0", APIVersion: agentplugin.CurrentAPIVersion, Type: agentplugin.PluginTypeModelProvider}
}
func (p *testModelPluginB) Init(context.Context) error { return nil }
func (p *testModelPluginB) Close() error               { return nil }
func (p *testModelPluginB) Model() (agentmodel.Model, error) {
	return p.m, nil
}

type testToolPlugin struct{ tools []agentcore.Tool }

func (p *testToolPlugin) Manifest() agentplugin.PluginManifest {
	return agentplugin.PluginManifest{Name: "tools-x", Version: "1.0.0", APIVersion: agentplugin.CurrentAPIVersion, Type: agentplugin.PluginTypeTool, Permissions: []string{"file:read"}}
}
func (p *testToolPlugin) Init(context.Context) error { return nil }
func (p *testToolPlugin) Close() error               { return nil }
func (p *testToolPlugin) Tools() []agentcore.Tool    { return p.tools }

type fakeModel struct{ name string }

func (m *fakeModel) ModelName() string { return m.name }
func (m *fakeModel) Complete(_ context.Context, _ agentmodel.Request) (agentmodel.Response, error) {
	return agentmodel.Response{Message: agentmodel.Message{Role: agentmodel.RoleAssistant, Content: "hi"}}, nil
}
func (m *fakeModel) Stream(_ context.Context, _ agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	ch := make(chan agentmodel.StreamEvent, 2)
	ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventDelta, Delta: "hi"}
	ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop}
	close(ch)
	return ch, nil
}

func newPlugins(t *testing.T) *agentplugin.Registry {
	t.Helper()
	reg := agentplugin.NewRegistry()
	if err := reg.Register(context.Background(), &testToolPlugin{tools: []agentcore.Tool{&testTool{name: "t1"}, &testTool{name: "t2"}}}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(context.Background(), &testModelPlugin{m: &fakeModel{name: "fm"}}); err != nil {
		t.Fatal(err)
	}
	return reg
}

// ---- Manifest 校验 ----

func TestValidate_OK(t *testing.T) {
	m := &AgentManifest{APIVersion: "v1", Kind: "agent", Name: "a", Model: ModelSpec{Provider: "model-a", Model: "m"}}
	if err := m.Validate(); err != nil {
		t.Fatalf("应通过: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name string
		m    *AgentManifest
		code string
	}{
		{"apiVersion", &AgentManifest{APIVersion: "v2", Name: "a", Model: ModelSpec{Provider: "p", Model: "m"}}, "unsupported_api_version"},
		{"kind", &AgentManifest{APIVersion: "v1", Kind: "graph", Name: "a", Model: ModelSpec{Provider: "p", Model: "m"}}, "invalid_kind"},
		{"name", &AgentManifest{APIVersion: "v1", Name: "", Model: ModelSpec{Provider: "p", Model: "m"}}, "missing_name"},
		{"provider", &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Model: "m"}}, "missing_provider"},
		{"model name", &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Provider: "p"}}, "missing_model_name"},
	}
	for _, cse := range cases {
		err := cse.m.Validate()
		var me *ManifestError
		if err == nil || !asManifestErr(err, &me) || me.Code != cse.code {
			t.Fatalf("%s: 期望 %s, 实际 %v", cse.name, cse.code, err)
		}
	}
}

func asManifestErr(err error, target **ManifestError) bool {
	me, ok := err.(*ManifestError)
	if ok {
		*target = me
	}
	return ok
}

// ---- Parse / Load ----

func TestParseManifest_YAML(t *testing.T) {
	yaml := `
api_version: v1
kind: agent
name: demo
model:
  provider: model-a
  model: gpt-5.4
tools:
  - t1
  - t2
`
	m, err := ParseManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("解析: %v", err)
	}
	if m.Name != "demo" || m.Model.Provider != "model-a" || len(m.Tools) != 2 {
		t.Fatalf("解析结果=%+v", m)
	}
}

func TestLoadManifest_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agent.yaml")
	content := "api_version: v1\nkind: agent\nname: f\nmodel:\n  provider: model-a\n  model: m\ntools:\n  - t1\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if m.Name != "f" {
		t.Fatalf("name=%q", m.Name)
	}
}

// ---- Compose ----

func TestCompose_Success(t *testing.T) {
	reg := newPlugins(t)
	m := &AgentManifest{APIVersion: "v1", Kind: "agent", Name: "demo", Model: ModelSpec{Provider: "model-a", Model: "m"}, Tools: []string{"t1", "t2"}}
	comp, err := Compose(context.Background(), m, reg, nil)
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if comp.Agent == nil {
		t.Fatal("Agent 为空")
	}
	if len(comp.Tools) != 2 {
		t.Fatalf("Tools=%d 期望 2", len(comp.Tools))
	}
	// 组装后的 Agent 可运行（用 fake model）
	res, err := comp.Agent.Run(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.FinalMessage.Content != "hi" {
		t.Fatalf("结果=%q", res.FinalMessage.Content)
	}
}

func TestCompose_UnknownTool(t *testing.T) {
	reg := newPlugins(t)
	m := &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Provider: "model-a", Model: "m"}, Tools: []string{"missing"}}
	_, err := Compose(context.Background(), m, reg, nil)
	var me *ManifestError
	if !asManifestErr(err, &me) || me.Code != "unknown_tool" {
		t.Fatalf("期望 unknown_tool, 实际 %v", err)
	}
}

func TestCompose_UnknownProvider(t *testing.T) {
	reg := newPlugins(t)
	m := &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Provider: "nope", Model: "m"}}
	_, err := Compose(context.Background(), m, reg, nil)
	var me *ManifestError
	if !asManifestErr(err, &me) || me.Code != "unknown_provider" {
		t.Fatalf("期望 unknown_provider, 实际 %v", err)
	}
}

func TestCompose_OpenAICompatible(t *testing.T) {
	reg := newPlugins(t)
	m := &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Provider: "openai-compatible", BaseURL: "https://x", APIKey: "${MTS_API_KEY}", Model: "gpt-5.4"}, Tools: []string{"t1"}}
	comp, err := Compose(context.Background(), m, reg, &fakeModel{name: "openai"})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if comp.Agent == nil {
		t.Fatal("Agent 为空")
	}
	if m.Model.ResolveAPIKey() != os.Getenv("MTS_API_KEY") {
		t.Fatalf("env 引用解析失败: %q", m.Model.ResolveAPIKey())
	}
}

// ---- Builder ----

func TestBuilder_Build(t *testing.T) {
	a, err := NewBuilder().
		Name("demo").
		Model(&fakeModel{name: "fm"}).
		Tools(&testTool{name: "t1"}, &testTool{name: "t2"}).
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if a == nil {
		t.Fatal("Agent 为空")
	}
	res, err := a.Run(context.Background(), "x")
	if err != nil || res.FinalMessage.Content != "hi" {
		t.Fatalf("Run: %v %v", res, err)
	}
}

func TestBuilder_MissingModel(t *testing.T) {
	if _, err := NewBuilder().Name("x").Build(); err == nil {
		t.Fatal("缺 model 应报错")
	}
}

func TestBuilder_DuplicateTool(t *testing.T) {
	_, err := NewBuilder().Model(&fakeModel{}).Tools(&testTool{name: "dup"}, &testTool{name: "dup"}).Build()
	var me *ManifestError
	if !asManifestErr(err, &me) || me.Code != "duplicate_tool" {
		t.Fatalf("期望 duplicate_tool, 实际 %v", err)
	}
}

// TestCompose_ProviderReplacement 验证 B2：替换 Model Provider 插件
// 不改 agent-core 源码（两个 provider 插件分别 Compose 出同一结构 Agent）。
func TestCompose_ProviderReplacement(t *testing.T) {
	reg := agentplugin.NewRegistry()
	if err := reg.Register(context.Background(), &testToolPlugin{tools: []agentcore.Tool{&testTool{name: "t1"}}}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(context.Background(), &testModelPlugin{m: &fakeModel{name: "provider-a"}}); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(context.Background(), &testModelPluginB{m: &fakeModel{name: "provider-b"}}); err != nil {
		t.Fatal(err)
	}

	base := AgentManifest{APIVersion: "v1", Name: "same-agent", Tools: []string{"t1"}}
	for _, provider := range []string{"model-a", "model-b"} {
		m := base
		m.Model = ModelSpec{Provider: provider, Model: "m"}
		comp, err := Compose(context.Background(), &m, reg, nil)
		if err != nil {
			t.Fatalf("provider %s Compose 失败: %v", provider, err)
		}
		res, err := comp.Agent.Run(context.Background(), "x")
		if err != nil || res.FinalMessage.Content != "hi" {
			t.Fatalf("provider %s Run 失败: %v %v", provider, res, err)
		}
	}
}

// ---- Marshal 脱敏 ----

func TestMarshalJSON_RedactsAPIKey(t *testing.T) {
	m := &AgentManifest{APIVersion: "v1", Name: "a", Model: ModelSpec{Provider: "openai-compatible", BaseURL: "https://x", APIKey: "sk-super-secret", Model: "m"}}
	out, err := m.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if strings.Contains(s, "sk-super-secret") {
		t.Fatalf("APIKey 泄露: %s", s)
	}
	if !strings.Contains(s, "[REDACTED]") {
		t.Fatalf("应含 [REDACTED]: %s", s)
	}
}

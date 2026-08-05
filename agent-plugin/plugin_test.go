package agentplugin

import (
	"context"
	"errors"
	"testing"

	agentcore "github.com/mantis-layer/mts/agent-core"
	agentmodel "github.com/mantis-layer/mts/agent-model"
)

// ---- 假插件实现（契约测试） ----

type simpleTool struct{ name string }

func (t *simpleTool) Name() string        { return t.name }
func (t *simpleTool) Description() string { return "simple " + t.name }
func (t *simpleTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *simpleTool) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	return map[string]any{"ok": true}, nil
}

type fakeToolPlugin struct {
	manifest   PluginManifest
	tools      []agentcore.Tool
	initErr    error
	initCalled *bool
	closeCount *int
}

func (p *fakeToolPlugin) Manifest() PluginManifest { return p.manifest }
func (p *fakeToolPlugin) Init(_ context.Context) error {
	if p.initCalled != nil {
		*p.initCalled = true
	}
	return p.initErr
}
func (p *fakeToolPlugin) Close() error {
	if p.closeCount != nil {
		*p.closeCount++
	}
	return nil
}
func (p *fakeToolPlugin) Tools() []agentcore.Tool { return p.tools }

type fakeModelPlugin struct {
	manifest PluginManifest
	model    agentmodel.Model
}

func (p *fakeModelPlugin) Manifest() PluginManifest { return p.manifest }
func (p *fakeModelPlugin) Init(_ context.Context) error { return nil }
func (p *fakeModelPlugin) Close() error                 { return nil }
func (p *fakeModelPlugin) Model() (agentmodel.Model, error) {
	if p.model == nil {
		return nil, errors.New("no model")
	}
	return p.model, nil
}

type fakeModel struct{ name string }

func (m *fakeModel) ModelName() string { return m.name }
func (m *fakeModel) Complete(_ context.Context, _ agentmodel.Request) (agentmodel.Response, error) {
	return agentmodel.Response{Message: agentmodel.Message{Role: agentmodel.RoleAssistant, Content: "hi"}}, nil
}
func (m *fakeModel) Stream(_ context.Context, _ agentmodel.Request) (<-chan agentmodel.StreamEvent, error) {
	ch := make(chan agentmodel.StreamEvent, 1)
	ch <- agentmodel.StreamEvent{Kind: agentmodel.StreamEventFinish, FinishReason: agentmodel.FinishReasonStop}
	close(ch)
	return ch, nil
}

func validManifest(name string, typ PluginType) PluginManifest {
	return PluginManifest{
		Name:       name,
		Version:    "1.0.0",
		APIVersion: CurrentAPIVersion,
		Type:       typ,
		Permissions: []string{"file:read"},
	}
}

// ---- ValidateManifest ----

func TestValidateManifest_OK(t *testing.T) {
	for _, typ := range []PluginType{PluginTypeTool, PluginTypeModelProvider, PluginTypeEvaluator, PluginTypePattern} {
		if err := ValidateManifest(validManifest("p", typ)); err != nil {
			t.Fatalf("类型 %s 应通过: %v", typ, err)
		}
	}
}

func TestValidateManifest_Errors(t *testing.T) {
	cases := []struct {
		name string
		m    PluginManifest
		code string
	}{
		{"空 name", PluginManifest{Name: "", Version: "1.0.0", APIVersion: "v1", Type: PluginTypeTool}, "missing_name"},
		{"缺 apiVersion", PluginManifest{Name: "p", Version: "1.0.0", Type: PluginTypeTool}, "missing_api_version"},
		{"apiVersion 不兼容", PluginManifest{Name: "p", Version: "1.0.0", APIVersion: "v2", Type: PluginTypeTool}, "unsupported_api_version"},
		{"未知类型", PluginManifest{Name: "p", Version: "1.0.0", APIVersion: "v1", Type: "wat"}, "unknown_type"},
		{"非法版本", PluginManifest{Name: "p", Version: "abc", APIVersion: "v1", Type: PluginTypeTool}, "invalid_version"},
		{"缺版本", PluginManifest{Name: "p", APIVersion: "v1", Type: PluginTypeTool}, "missing_version"},
	}
	for _, cse := range cases {
		err := ValidateManifest(cse.m)
		if err == nil {
			t.Fatalf("%s: 应报错", cse.name)
		}
		var me *ManifestError
		if !errors.As(err, &me) || me.Code != cse.code {
			t.Fatalf("%s: 期望 code=%s, 实际 %v", cse.name, cse.code, err)
		}
	}
}

// ---- Registry ----

func TestRegistry_RegisterToolPlugin(t *testing.T) {
	r := NewRegistry()
	initCalled := false
	p := &fakeToolPlugin{
		manifest:   validManifest("tools-a", PluginTypeTool),
		tools:      []agentcore.Tool{&simpleTool{name: "t1"}, &simpleTool{name: "t2"}},
		initCalled: &initCalled,
	}
	if err := r.Register(context.Background(), p); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if !initCalled {
		t.Fatal("Init 未被调用")
	}
	if _, ok := r.GetTool("t1"); !ok {
		t.Fatal("GetTool(t1) 失败")
	}
	if len(r.Tools()) != 2 {
		t.Fatalf("Tools=%d", len(r.Tools()))
	}
	if len(r.ToolSchemas()) != 2 {
		t.Fatalf("ToolSchemas=%d", len(r.ToolSchemas()))
	}
	// 权限可查询（B4）
	if got, _ := r.GetPlugin("tools-a"); len(got.Manifest().Permissions) == 0 || got.Manifest().Permissions[0] != "file:read" {
		t.Fatalf("权限声明丢失: %+v", got.Manifest())
	}
}

func TestRegistry_RegisterModelProviderPlugin(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(context.Background(), &fakeModelPlugin{manifest: validManifest("provider-a", PluginTypeModelProvider), model: &fakeModel{name: "m1"}}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	m, ok := r.GetModel("provider-a")
	if !ok || m == nil {
		t.Fatal("GetModel 失败")
	}
	if len(r.Models()) != 1 {
		t.Fatalf("Models=%d", len(r.Models()))
	}
}

func TestRegistry_DuplicateName(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("dup", PluginTypeTool)}); err != nil {
		t.Fatal(err)
	}
	err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("dup", PluginTypeTool)})
	var me *ManifestError
	if !errors.As(err, &me) || me.Code != "duplicate_name" {
		t.Fatalf("重复注册应报 duplicate_name, 实际 %v", err)
	}
}

func TestRegistry_InitFailureNotRegistered(t *testing.T) {
	r := NewRegistry()
	err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("bad", PluginTypeTool), initErr: errors.New("boom")})
	if err == nil {
		t.Fatal("Init 失败应报错")
	}
	if _, ok := r.GetPlugin("bad"); ok {
		t.Fatal("Init 失败不应残留注册")
	}
}

func TestRegistry_TypeMismatch(t *testing.T) {
	r := NewRegistry()
	// 声明 type=tool 但未实现 ToolPlugin
	if err := r.Register(context.Background(), &fakeModelPlugin{manifest: validManifest("mismatch", PluginTypeTool), model: &fakeModel{name: "m"}}); err == nil {
		t.Fatal("类型不匹配应报错")
	}
	// 失败后不得残留（blocking bug 回归）
	if _, ok := r.GetPlugin("mismatch"); ok {
		t.Fatal("类型不匹配后插件不应残留")
	}
	if len(r.Plugins()) != 0 || len(r.Tools()) != 0 || len(r.Models()) != 0 {
		t.Fatal("类型不匹配后注册表不应有任何残留")
	}
}

func TestRegistry_ToolConflictNoResidue(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("first", PluginTypeTool), tools: []agentcore.Tool{&simpleTool{name: "shared"}}}); err != nil {
		t.Fatal(err)
	}
	// 第二个插件提供同名工具 → 提交阶段查重失败，不应残留
	err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("second", PluginTypeTool), tools: []agentcore.Tool{&simpleTool{name: "shared"}}})
	var me *ManifestError
	if !errors.As(err, &me) || me.Code != "duplicate_tool" {
		t.Fatalf("期望 duplicate_tool, 实际 %v", err)
	}
	if _, ok := r.GetPlugin("second"); ok {
		t.Fatal("冲突插件不应残留")
	}
	if len(r.Tools()) != 1 {
		t.Fatalf("工具数=%d 期望 1（仅 first）", len(r.Tools()))
	}
}

func TestRegistry_CloseAll(t *testing.T) {
	r := NewRegistry()
	var closeCount int
	if err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("a", PluginTypeTool), closeCount: &closeCount}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(context.Background(), &fakeToolPlugin{manifest: validManifest("b", PluginTypeTool), closeCount: &closeCount}); err != nil {
		t.Fatal(err)
	}
	if err := r.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if closeCount != 2 {
		t.Fatalf("Close 调用次数=%d 期望 2", closeCount)
	}
}

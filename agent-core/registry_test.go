package agentcore

import (
	"context"
	"testing"
)

type simpleTool struct{ name string }

func (t *simpleTool) Name() string        { return t.name }
func (t *simpleTool) Description() string { return "simple " + t.name }
func (t *simpleTool) Parameters() map[string]any {
	return map[string]any{"type": "object"}
}
func (t *simpleTool) Execute(_ context.Context, _ map[string]any) (map[string]any, error) {
	return nil, nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&simpleTool{name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Get("a"); !ok {
		t.Fatal("Get(a) 失败")
	}
	if _, ok := r.Get("missing"); ok {
		t.Fatal("Get(missing) 不应命中")
	}
	if r.Len() != 1 {
		t.Fatalf("Len=%d 期望 1", r.Len())
	}
}

func TestRegistry_DuplicateNameRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&simpleTool{name: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&simpleTool{name: "a"}); err == nil {
		t.Fatal("重复注册应报错")
	}
}

func TestRegistry_EmptyNameRejected(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(&simpleTool{name: ""}); err == nil {
		t.Fatal("空 Name 应报错")
	}
}

func TestRegistry_Schemas(t *testing.T) {
	r := NewRegistry()
	_ = r.Register(&simpleTool{name: "a"})
	schemas := r.Schemas()
	if len(schemas) != 1 || schemas[0].Function.Name != "a" {
		t.Fatalf("Schemas=%+v", schemas)
	}
}

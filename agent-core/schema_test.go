package agentcore

import "testing"

func TestValidateSchema_OK(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"n":    map[string]any{"type": "integer"},
		},
		"required": []string{"path"},
	}
	if err := ValidateJSONSchema(schema, map[string]any{"path": "/tmp/a.json", "n": 3}); err != nil {
		t.Fatalf("应通过: %v", err)
	}
}

func TestValidateSchema_MissingRequired(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []string{"path"},
	}
	if err := ValidateJSONSchema(schema, map[string]any{}); err == nil {
		t.Fatal("缺少必填字段应报错")
	}
}

func TestValidateSchema_WrongType(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"count": map[string]any{"type": "integer"},
		},
	}
	if err := ValidateJSONSchema(schema, map[string]any{"count": "abc"}); err == nil {
		t.Fatal("类型不匹配应报错")
	}
	// float 3.0 作为 integer 应通过
	if err := ValidateJSONSchema(schema, map[string]any{"count": 3.0}); err != nil {
		t.Fatalf("3.0 应视为 integer 通过: %v", err)
	}
}

func TestValidateSchema_Enum(t *testing.T) {
	schema := map[string]any{
		"type": "string",
		"enum": []any{"a", "b"},
	}
	if err := ValidateJSONSchema(schema, "a"); err != nil {
		t.Fatalf("enum 命中应通过: %v", err)
	}
	if err := ValidateJSONSchema(schema, "c"); err == nil {
		t.Fatal("enum 未命中应报错")
	}
}

func TestValidateSchema_EnumWithNestedObject(t *testing.T) {
	// 回归：enum 命中后不得跳过嵌套 object 校验
	schema := map[string]any{
		"type": "object",
		"enum": []any{},
		"properties": map[string]any{
			"mode": map[string]any{"type": "string", "enum": []any{"fast", "safe"}},
		},
		"required": []string{"mode"},
	}
	// enum 为空时不做 enum 约束，但嵌套校验仍须执行
	if err := ValidateJSONSchema(schema, map[string]any{"mode": "fast"}); err != nil {
		t.Fatalf("合法值应通过: %v", err)
	}
	if err := ValidateJSONSchema(schema, map[string]any{"mode": "slow"}); err == nil {
		t.Fatal("嵌套 enum 未命中应报错")
	}
	if err := ValidateJSONSchema(schema, map[string]any{}); err == nil {
		t.Fatal("缺少必填字段应报错")
	}
}

func TestValidateSchema_ArrayItems(t *testing.T) {
	schema := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
	if err := ValidateJSONSchema(schema, []any{"x", "y"}); err != nil {
		t.Fatalf("应通过: %v", err)
	}
	if err := ValidateJSONSchema(schema, []any{"x", 3}); err == nil {
		t.Fatal("数组元素类型错误应报错")
	}
}

func TestValidateSchema_MinMax(t *testing.T) {
	schema := map[string]any{"type": "number", "minimum": 0.0, "maximum": 100.0}
	if err := ValidateJSONSchema(schema, 50.0); err != nil {
		t.Fatalf("范围内应通过: %v", err)
	}
	if err := ValidateJSONSchema(schema, 150.0); err == nil {
		t.Fatal("超出最大值应报错")
	}
}

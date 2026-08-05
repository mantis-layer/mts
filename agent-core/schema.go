package agentcore

import (
	"fmt"
	"math"
)

// ValidateJSONSchema 校验 value 是否符合简化 JSON Schema 子集：
// type / required / properties / items / enum / minimum / maximum。
// 覆盖 Tool 输入输出校验所需的最小子集，避免引入第三方 Schema 依赖。
func ValidateJSONSchema(schema map[string]any, value any) error {
	if schema == nil {
		return nil
	}
	return validateValue(schema, value, "$")
}

func validateValue(schema map[string]any, value any, path string) error {
	typ, _ := schema["type"].(string)
	if typ != "" {
		if err := checkType(typ, value, path); err != nil {
			return err
		}
	}
	if enum, ok := schema["enum"].([]any); ok && len(enum) > 0 {
		for _, e := range enum {
			if fmt.Sprintf("%v", e) == fmt.Sprintf("%v", value) {
				return nil
			}
		}
		return fmt.Errorf("%s: 值不在 enum 允许范围内: %v", path, value)
	}
	switch typ {
	case "object":
		props, _ := schema["properties"].(map[string]any)
		obj, ok := value.(map[string]any)
		if !ok {
			if value == nil {
				return nil
			}
			return fmt.Errorf("%s: 期望 object，实际 %T", path, value)
		}
		if reqRaw, ok := schema["required"]; ok {
			for _, name := range toStringSlice(reqRaw) {
				if _, present := obj[name]; !present {
					return fmt.Errorf("%s: 缺少必填字段 %q", path, name)
				}
			}
		}
		for name, v := range obj {
			childPath := path + "." + name
			if sub, ok := props[name].(map[string]any); ok {
				if err := validateValue(sub, v, childPath); err != nil {
					return err
				}
			}
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			if value == nil {
				return nil
			}
			return fmt.Errorf("%s: 期望 array，实际 %T", path, value)
		}
		if items, ok := schema["items"].(map[string]any); ok {
			for i, item := range arr {
				if err := validateValue(items, item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	case "number", "integer":
		if f, ok := toFloat(value); ok {
			if min, ok := schema["minimum"].(float64); ok && f < min {
				return fmt.Errorf("%s: 小于最小值 %v", path, min)
			}
			if max, ok := schema["maximum"].(float64); ok && f > max {
				return fmt.Errorf("%s: 大于最大值 %v", path, max)
			}
		}
	}
	return nil
}

func checkType(typ string, value any, path string) error {
	switch typ {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: 期望 string，实际 %T", path, value)
		}
	case "number":
		if _, ok := toFloat(value); !ok {
			return fmt.Errorf("%s: 期望 number，实际 %T", path, value)
		}
	case "integer":
		if _, ok := toInt(value); !ok {
			return fmt.Errorf("%s: 期望 integer，实际 %T", path, value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: 期望 boolean，实际 %T", path, value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("%s: 期望 object，实际 %T", path, value)
		}
	case "array":
		if _, ok := value.([]any); !ok {
			return fmt.Errorf("%s: 期望 array，实际 %T", path, value)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s: 期望 null，实际 %T", path, value)
		}
	}
	return nil
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		if n == math.Trunc(n) {
			return int64(n), true
		}
	}
	return 0, false
}

// toStringSlice 将 []string 或 []any（元素为 string）统一转为 []string。
func toStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

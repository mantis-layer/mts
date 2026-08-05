package agentmodel

// ToolSchema 描述一个可被模型调用的工具（OpenAI 兼容 tools 数组元素）。
type ToolSchema struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

// FunctionSchema 描述 function 类型工具的名称、说明与参数 JSON Schema。
type FunctionSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema
}

// NewToolSchema 构造一个 function 类型工具 Schema。
func NewToolSchema(name, description string, parameters map[string]any) ToolSchema {
	return ToolSchema{
		Type: "function",
		Function: FunctionSchema{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}
}

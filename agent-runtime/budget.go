package agentruntime

// Budget 是 TaskRun 的执行预算（FR-004）。
type Budget struct {
	// MaxIterations 模型调用轮次上限；0 表示不限。
	MaxIterations int
	// MaxToolCalls 工具调用次数上限；0 表示不限。
	MaxToolCalls int
}

// Exceeded 判断当前消耗是否超出预算。
func (b Budget) Exceeded(iterations, toolCalls int) bool {
	if b.MaxIterations > 0 && iterations >= b.MaxIterations {
		return true
	}
	if b.MaxToolCalls > 0 && toolCalls >= b.MaxToolCalls {
		return true
	}
	return false
}

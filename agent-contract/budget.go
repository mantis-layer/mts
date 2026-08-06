package agentcontract

// Budget 是 TaskRun 的执行预算（FR-004）。
type Budget struct {
	// MaxIterations 模型调用轮次上限；0 表示不限。
	MaxIterations int
	// MaxToolCalls 工具调用次数上限；0 表示不限。
	MaxToolCalls int
}

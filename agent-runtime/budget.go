package agentruntime

import contract "github.com/mantis-layer/mts/agent-contract"

// Budget 上移至 agent-contract；runtime 通过 type alias 保持 API 兼容。
type Budget = contract.Budget

// budgetExceeded 判断当前消耗是否超出预算（原 Budget.Exceeded 方法，内化为函数）。
func budgetExceeded(b Budget, iterations, toolCalls int) bool {
	if b.MaxIterations > 0 && iterations >= b.MaxIterations {
		return true
	}
	if b.MaxToolCalls > 0 && toolCalls >= b.MaxToolCalls {
		return true
	}
	return false
}

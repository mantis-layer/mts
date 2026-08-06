package agentruntime

import contract "github.com/mantis-layer/mts/agent-contract"

// EventKind 上移至 agent-contract；runtime 通过 type alias 保持 API 兼容。
type EventKind = contract.EventKind

// RuntimeEvent 上移至 agent-contract。
type RuntimeEvent = contract.RuntimeEvent

// Re-export constants。
const (
	EventTaskRunCreated      = contract.EventTaskRunCreated
	EventTaskRunStarted      = contract.EventTaskRunStarted
	EventStateChanged        = contract.EventStateChanged
	EventTaskRunCompleted    = contract.EventTaskRunCompleted
	EventTaskRunFailed       = contract.EventTaskRunFailed
	EventTaskRunCancelled    = contract.EventTaskRunCancelled
	EventBudgetExceeded      = contract.EventBudgetExceeded
	EventHumanInputRequested = contract.EventHumanInputRequested
	EventHumanInputReceived  = contract.EventHumanInputReceived
	EventCheckpointSaved     = contract.EventCheckpointSaved
	EventArtifactCreated     = contract.EventArtifactCreated
	EventEvaluatorResult     = contract.EventEvaluatorResult
)

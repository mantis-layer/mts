package agentcontract

import "time"

// EventKind 标识 Runtime 事件类型（FR-009 Observability）。
type EventKind string

const (
	EventTaskRunCreated      EventKind = "taskrun.created"
	EventTaskRunStarted      EventKind = "taskrun.started"
	EventStateChanged        EventKind = "taskrun.state_changed"
	EventTaskRunCompleted    EventKind = "taskrun.completed"
	EventTaskRunFailed       EventKind = "taskrun.failed"
	EventTaskRunCancelled    EventKind = "taskrun.cancelled"
	EventBudgetExceeded      EventKind = "taskrun.budget_exceeded"
	EventHumanInputRequested EventKind = "taskrun.human_input_requested"
	EventHumanInputReceived  EventKind = "taskrun.human_input_received"
	EventCheckpointSaved     EventKind = "taskrun.checkpoint_saved"
	EventArtifactCreated     EventKind = "artifact.created"
	EventEvaluatorResult     EventKind = "taskrun.evaluator_result"
)

// RuntimeEvent 是一次 Runtime 事件。
type RuntimeEvent struct {
	ID        string
	TaskRunID string
	Kind      EventKind
	Data      map[string]any
	Timestamp time.Time
}

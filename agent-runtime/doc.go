// Package agentruntime 提供通用 Task Runtime（FR-004）：
// Task/TaskRun、Run 状态机、预算、取消、事件订阅、Artifact/Evidence、
// Evaluator、基础 Checkpoint 持久化恢复与 Human-in-the-loop。
//
// Pattern 只决定下一步行为（FR-005），不负责持久化、工具执行与预算统计。
//
// 纯数据协议类型（Task、TaskRun、RunState、Artifact、Budget、Event、StepResult 等）
// 已上移至 agent-contract 模块（架构 §6.3）；本包通过 type alias 保持 API 兼容。
package agentruntime

// Package agentcontract 提供通用 Agent Task 的纯数据协议类型（架构 §6.3）。
//
// 该模块仅包含纯数据/枚举定义，不包含状态机、持久化、执行逻辑。
// 行为方法（状态转换、克隆、预算检查等）由 agent-runtime 作为内部函数提供。
//
// 依赖方向：agent-contract → agent-model（统一 Usage 类型）
package agentcontract

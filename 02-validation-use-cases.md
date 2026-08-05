# Agent Platform 验证场景 v0.1

> 目的：不是交付三个完整产品，而是用三条垂直切片验证 Runtime 的抽象是否成立。

## 1. 验证原则

每个场景必须：

- 使用相同的 `agent-model`、`agent-core` 和 `agent-runtime`
- 不修改 Runtime Core
- 通过不同模块组合体现不同执行模式
- 产生结构化 Event、Artifact 和 TaskResult
- 具备明确验收标准
- 包含至少一个失败或中断路径

---

# 场景 A：Tool Loop Agent

## 2. 目标

验证最小 Agent Loop 是否能可靠完成：

```text
用户请求
→ 模型决定调用工具
→ Tool 返回结果
→ 模型继续决策
→ 输出最终结果
```

## 3. 示例任务

> 读取一份结构化数据，计算指定指标，并输出摘要。

可使用本地 JSON / CSV 输入，避免把网络可用性变成核心测试前提。

## 4. 模块组合

```text
Model Provider
+ Agent Core
+ File Reader Tool
+ Calculator Tool
+ Tool Loop Pattern
+ In-memory Artifact Store
```

## 5. 执行过程

1. 用户提交 Task。
2. Runtime 创建 TaskRun。
3. Agent Core 调用模型。
4. 模型调用 File Reader。
5. 模型根据数据调用 Calculator。
6. Tool 结果写入消息上下文。
7. 模型生成摘要。
8. Runtime 保存结果 Artifact。
9. Evaluator 校验输出 Schema。

## 6. 预期产物

- `data.summary`：结构化摘要
- `execution.trace`：模型和工具调用记录
- `task.result`：任务状态、Usage 和验收结果

## 7. 验收标准

- 至少发生两次 Tool Call。
- Tool 输入输出通过 Schema 校验。
- Tool 超时可以被取消。
- 模型调用和 Tool 调用均产生事件。
- 最终摘要中的关键数字与 Calculator 结果一致。
- 超过预算后停止继续调用。

## 8. 验证模块

| 模块 | 验证点 |
|---|---|
| agent-model | 流式消息、Tool Call、Usage |
| agent-core | Model/Tool Loop、事件、取消 |
| agent-plugin | Tool 静态注册 |
| agent-runtime | TaskRun、Budget、Artifact |
| evaluator | Schema 和数值一致性 |

## 9. 故障路径

- 文件不存在
- Tool 返回非法 Schema
- 模型重复调用相同 Tool
- Tool 超时
- Token 或 Tool Call 预算耗尽

---

# 场景 B：Research Agent

## 10. 目标

验证多步骤、长上下文、证据管理和结果验收。

## 11. 示例任务

> 针对一个技术主题收集多个来源，形成结构化研究报告，关键结论必须绑定证据。

## 12. 模块组合

```text
Research Pattern
+ Search Tool
+ Document Reader
+ Context Builder
+ Artifact Store
+ Citation / Evidence Evaluator
+ SQLite Run Store
```

## 13. 执行过程

1. 接收研究目标和约束。
2. Research Pattern 生成问题拆解。
3. 调用 Search Tool 获取候选来源。
4. 读取与筛选资料。
5. 保存 Source Artifact 和 Evidence。
6. Context Builder 只加载当前步骤相关材料。
7. 生成报告草稿。
8. Evaluator 检查关键结论的证据覆盖率。
9. 覆盖不足时继续补充检索，或在预算耗尽时标记未解决问题。
10. 输出最终报告和证据表。

## 14. 预期产物

- `research.plan`
- `source.collection`
- `evidence.table`
- `research.report`
- `evaluation.report`

## 15. 验收标准

- 报告包含明确结构。
- 关键结论都能引用 Evidence。
- 来源数量达到任务要求。
- 相同来源不会被重复计数。
- Context 不直接塞入全部原始资料。
- 预算耗尽时返回部分结果和缺口，而不是伪装成功。
- Run 重启后能从最近 Checkpoint 继续。

## 16. 验证模块

| 模块 | 验证点 |
|---|---|
| Pattern Host | Research Pattern 驱动多步骤执行 |
| Context Builder | 检索、压缩和上下文预算 |
| Artifact Manager | 来源、证据和报告 |
| Evaluator | Evidence Coverage |
| Storage | SQLite 持久化 |
| Runtime | 重试、Checkpoint、部分成功 |

## 17. 故障路径

- 搜索工具不可用
- 来源内容冲突
- 证据覆盖不足
- 上下文超限
- 模型输出无有效引用
- Runtime 中途重启

---

# 场景 C：Workflow Agent

## 18. 目标

验证确定性流程、人工审批、持久化和恢复。

## 19. 示例任务

> 接收一份变更申请，执行规则检查，生成风险摘要，等待人工批准后继续执行下一步。

该场景不绑定具体行业，可用模拟审批流程。

## 20. 模块组合

```text
Workflow Pattern
+ Rule Evaluator
+ Human Approval
+ Policy Engine
+ SQLite Run Store
+ Audit Log
```

## 21. 执行过程

1. 接收申请 Artifact。
2. 执行格式和必填项检查。
3. 调用模型生成风险摘要。
4. Rule Evaluator 执行确定性规则。
5. 高风险申请进入 `WAITING_HUMAN`。
6. Runtime 保存 Checkpoint 并停止消耗模型预算。
7. 人工通过 API / CLI 提交批准或拒绝。
8. Runtime 恢复。
9. 批准则执行后续 Tool；拒绝则生成拒绝结果。
10. 保存完整审计记录。

## 22. 预期产物

- `request.normalized`
- `risk.summary`
- `rule.evaluation`
- `approval.record`
- `workflow.result`
- `audit.log`

## 23. 验收标准

- 状态机转换合法。
- 等待人工期间进程可退出。
- 重启后 Run 仍处于等待状态。
- 非授权用户不能批准。
- 人工决定不可被 Agent 覆盖。
- 每个状态变化都有审计事件。
- 审批后只能继续一次，不重复执行副作用 Tool。

## 24. 验证模块

| 模块 | 验证点 |
|---|---|
| Workflow Pattern | 确定性状态推进 |
| Policy Engine | 权限和风险规则 |
| Human Gateway | 暂停、批准、拒绝 |
| Checkpoint | 跨进程恢复 |
| Audit | 不可遗漏的状态事件 |
| Tool Runtime | 幂等或去重执行 |

## 25. 故障路径

- 非法状态跳转
- 未授权审批
- 批准消息重复发送
- Runtime 在执行副作用 Tool 后崩溃
- 持久化写入失败
- 人工长期不响应

---

# 26. 跨场景需求矩阵

| 能力 | Tool Loop | Research | Workflow |
|---|---:|---:|---:|
| Model Abstraction | 必须 | 必须 | 必须 |
| Agent Core | 必须 | 必须 | 可使用 |
| Tool API | 必须 | 必须 | 必须 |
| Pattern Host | 基础 | 必须 | 必须 |
| Task / Run | 必须 | 必须 | 必须 |
| Artifact | 必须 | 必须 | 必须 |
| Evaluator | Schema | Evidence | Rule / Human |
| Budget | 必须 | 必须 | 必须 |
| Checkpoint | 可选 | 必须 | 必须 |
| Human-in-the-loop | 否 | 可选 | 必须 |
| SQLite | 可选 | 必须 | 必须 |
| Audit | 基础 | 必须 | 必须 |
| 插件替换 | Tool | Tool / Evaluator | Policy / Storage |

## 27. 架构验证结论标准

当三个场景都跑通后，才能确认：

- Agent Core 与 Task Runtime 的分层是否合理
- Pattern 接口是否足够通用
- Artifact / Evidence 模型是否自然
- Plugin API 是否只暴露必要扩展点
- Task Contract 是否需要增加或删除字段
- Checkpoint 是否应该进入核心 Runtime
- 哪些功能仍应留在应用层

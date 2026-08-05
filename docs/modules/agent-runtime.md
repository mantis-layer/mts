# agent-runtime

`agent-runtime` 为长生命周期任务提供状态、存储、Checkpoint、审批和验收。它不实现具体模型或业务工具。

## 状态机

```text
created → running → completed
                  ↘ failed
                  ↘ cancelled
                  ↘ waiting → running
                            ↘ cancelled / failed
```

终态不可再次迁移。状态更新使用 Compare-and-Set 语义，避免并发 `Cancel` 覆盖正在执行的 Checkpoint。

## 使用步骤

1. 用 `NewRuntime(storage, budget)` 创建 Runtime。
2. 注册一个或多个 Pattern 与 Evaluator。
3. `SubmitTask` 创建 `TaskRun`。
4. `Run` 执行到终态，或进入 `waiting`。
5. 对等待中的 Run 用 `SubmitHumanInput` 提交人工输入；用 `Cancel` 请求取消。

## 内置组件

- Storage：`MemoryStorage` 用于测试/临时使用；`SQLiteStorage` 用于本地持久化。
- Pattern：`ToolLoopPattern`、`ResearchPattern`、`WorkflowPattern`。
- Evaluator：`SchemaEvaluator` 检查指定 Artifact 是否为合法 JSON；`EvidenceCoverageEvaluator` 检查 Evidence 数量。
- 产物：`Artifact` 保存结构化输出，`Evidence` 绑定其来源引用。

`Pattern.Execute` 只返回 `StepResult`；Runtime 统一保存进度、增加预算、写事件、落库 Artifact/Evidence，并在完成前执行 Evaluator。

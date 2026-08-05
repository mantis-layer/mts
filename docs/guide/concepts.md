# 核心概念

## Agent 与 Tool Loop

`agent-core.Agent` 运行 Model → Tool → Model 循环。模型以流式事件生成文本或 Tool Call；每个 Tool Call 会先经过 JSON Schema 校验，再在带超时的上下文中执行。没有 Tool Call 时，模型消息即为最终结果。

## Task 与 TaskRun

`Task` 是用户提交的任务定义；`TaskRun` 是一次具体运行。`agent-runtime` 将运行状态、输入、进度、Usage、结果和错误持久化到 `Storage`，使同一任务可查询、取消或在人工输入后继续。

## Pattern

Pattern 只决定某次运行的下一步，Runtime 负责持久化、预算和状态迁移。仓库现有的 Pattern 包括：

- `tool_loop`：用 `agent-core` 完成一次工具循环。
- `research`：把研究过程拆为计划、收集、报告三个可保存的阶段。
- `workflow`：按确定性步骤推进，并可在需要人工审批时进入等待状态。

## Artifact、Evidence 与 Evaluator

Pattern 可以产生结构化 `Artifact`，并为 Artifact 关联来源 `Evidence`。任务完成前，Runtime 会依次运行已注册的 Evaluator；任一 Evaluator 不通过，Run 会以 `failed` 结束。

## Plugin 与 Manifest

Plugin 用于把模型 Provider、工具、Evaluator 或 Pattern 加入 Registry。`agent-compose` 则通过 YAML/JSON Manifest 或 Go Builder，把模型和工具组合成可运行的 Agent。

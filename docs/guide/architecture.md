# 架构与边界

MTS 保持依赖方向单向：抽象在下层，组合与应用在上层。

```text
应用 / CLI / 示例
        ↓
agent-compose      agent-runtime
        ↓                 ↓
agent-plugin ───→ agent-core
        ↓                 ↓
  adapter / MCP       agent-model
```

## 分层职责

| 层 | 负责 | 不负责 |
|---|---|---|
| `agent-model` | 消息、工具 Schema、流、Usage、Model 接口 | 任何 Provider 的 HTTP 实现 |
| `agent-core` | 最小 Tool Loop、事件、取消、参数校验 | Task 生命周期与数据库 |
| `agent-runtime` | TaskRun、状态机、Checkpoint、Artifact、Evaluator | 具体模型与业务工具 |
| `agent-plugin` | 插件契约、Registry、生命周期、MCP Tool Adapter | Go 原生动态插件加载 |
| `agent-compose` | Manifest 校验与 Agent 组装 | 具体模型 SDK |

## 何时使用哪个入口

- 只需一次可流式观察的工具调用循环：直接使用 `agent-core`。
- 需要取消、等待人工输入、持久化或产物验收：使用 `agent-runtime`。
- 需要替换模型或按声明组合工具：使用 `agent-plugin` 与 `agent-compose`。

当前版本仍是早期 v0.1，公共 API 尚未冻结。以代码和测试为当前行为的依据，产品愿景与路线图用于说明范围而非替代实现契约。

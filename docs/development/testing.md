# 测试与验证

MTS 的测试按模块放置，并且覆盖正常与关键故障路径。修改实现时，优先运行改动模块的测试。

```bash
go test ./agent-core
go test ./agent-runtime
go test ./agent-plugin/...
go test ./agent-compose
go test ./adapters/model-openai
go test ./tools
```

## 覆盖重点

| 模块 | 已覆盖的核心行为 |
|---|---|
| `agent-core` | 多工具调用、参数校验、超时、取消、Steering、Usage 汇总 |
| `agent-runtime` | 合法状态迁移、并发取消、人工输入、预算、恢复、Evaluator |
| `agent-plugin` | Manifest 校验、工具冲突、生命周期、MCP RPC |
| `agent-compose` | Manifest 解析、环境变量密钥引用、Provider/Tool 解析、脱敏序列化 |
| OpenAI 适配器 | 流式文本、Tool Call 拼装、错误映射、可选契约测试 |
| `tools` | 计算表达式、文件格式、路径限制、参数错误 |

## OpenAI 契约测试

`adapters/model-openai` 的契约测试（`TestContract*`）只有配置了真实端点环境变量后才请求外部端点；未配置时自动 SKIP：

```bash
cd adapters/model-openai
export MTS_BASEURL="https://..." MTS_API_KEY="..." MTS_MODEL="..."
go test -v -run TestContract ./...
```

契约测试还支持**第二端点**（`MTS_BASEURL2`/`MTS_API_KEY2`/`MTS_MODEL2`）用于验证"≥2 个 OpenAI 兼容端点通过相同测试"。把它视为 Provider 兼容性验证，而不是普通单元测试；受限网络下测试会以"端点网络不可达"SKIP（环境限制，非失败）。

## 文档站验证

每次更改文档导航或链接后运行：

```bash
npm run docs:build
```

VitePress 会在构建期检查内部 Markdown 链接。GitHub Pages 工作流使用同一命令构建，再上传静态产物。

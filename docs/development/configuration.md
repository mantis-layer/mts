# 配置参考

## CLI 环境变量

| 变量 | 作用 | 是否必需 |
|---|---|---|
| `MTS_BASEURL` | OpenAI 兼容 API 的 Base URL | 是，除非使用 `--baseurl` |
| `MTS_API_KEY` | API 密钥 | 是，除非使用 `--api-key` |
| `MTS_MODEL` | 模型名称 | 是，除非使用 `--model` |

CLI 会优先读取旗标，缺失时回退到环境变量。它还会从当前目录向上最多查找四层 `.env.local`，并在仓库根目录停止；已有环境变量不会被文件覆盖。

```bash
go run ./cli --baseurl "$MTS_BASEURL" --api-key "$MTS_API_KEY" \
  --model "$MTS_MODEL" --task "读取 examples/data.json 并生成摘要" --json
```

## Agent Manifest

`agent-compose` 支持 YAML 或 JSON。`api_key` 只允许 `${ENV_NAME}` 引用，以避免把明文密钥保存在配置中。

```yaml
api_version: v1
kind: agent
name: data-summary
model:
  provider: openai-compatible
  base_url: https://your-openai-compatible-endpoint/v1
  api_key: ${MTS_API_KEY}
  model: your-model
tools:
  - file_reader
  - calculator
pattern: tool_loop
permissions: []
```

当前实现中，`openai-compatible` 需要调用方传入 `ModelFactory`；其他 `model.provider` 名称从已注册的模型插件中解析。Manifest 中的 `pattern` 与 `permissions` 是声明字段，是否由宿主执行取决于所使用的组合入口。

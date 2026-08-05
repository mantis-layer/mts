# Tool Loop Agent 示例

本目录包含 Tool Loop 垂直切片的示例输入。

## 本地运行

```bash
cd cli
export MTS_BASEURL=<中转站或 OpenAI 兼容 baseurl>
export MTS_API_KEY=<你的 key>
export MTS_MODEL=<模型名>

# 读取 data.json → 计算销售额总和 → 输出摘要
go run . --task "读取 ../examples/data.json，用 calculator 计算 sales 字段的总和，然后输出一段中文摘要，说明总销售额是多少"
```

也可使用仓库根目录的 `.env.local`（已被 `.gitignore` 保护，不会提交）：

```bash
cd cli
go run . --task "..."
```

CLI 会自动加载同目录上级的 `.env.local`。

## 契约测试（真实端点）

```bash
cd adapters/model-openai
export MTS_BASEURL=... MTS_API_KEY=... MTS_MODEL=...
go test -v -run TestContract ./...
```

> 注意：部分受限网络（沙箱/代理 allowlist）可能无法直连中转站，此时契约测试会
> SKIP 并注明"网络不可达"，这是环境限制而非失败。

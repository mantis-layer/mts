# 本地开发

## 仓库结构

这是一个 Go workspace，而不是单一 Go module。各模块可独立被下游项目引用，根目录的 `go.work` 将它们用于本地联调。

```text
agent-model/              模型协议
agent-core/               最小 Agent Loop
agent-runtime/            Task 生命周期与存储
agent-plugin/             插件与 MCP
agent-compose/            Manifest 与 Builder
adapters/model-openai/    OpenAI 兼容适配器
tools/                    内置示例工具
cli/                      命令行入口
examples/                 垂直切片示例
```

## 准备环境

```bash
git clone https://github.com/mantis-layer/mts.git
cd mts
go version
go work sync
```

完成一项模块修改后，应在受影响模块内先运行其测试，再执行所有 workspace 模块的聚合验证：

```bash
go test ./agent-model ./agent-core ./agent-runtime ./agent-plugin/... \
  ./agent-compose ./adapters/model-openai ./tools ./cli ./examples/tool_loop_agent
```

## 预览文档站

```bash
npm install
npm run docs:dev
```

开发服务器会打印本地地址。生产构建使用：

```bash
npm run docs:build
npm run docs:preview
```

构建产物位于 `docs/.vitepress/dist`，已由 `.gitignore` 忽略，不应提交。

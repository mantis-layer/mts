# MTS 项目指引

## 文档同步规则

每次完成代码变更并合并到 `main` 后，**必须**将 VitePress 文档站点工作树同步到最新：

```bash
cd /Users/carver/workspace/mindcarver/mts-gitbook-docs && git pull --ff-only origin main
```

这确保文档站点（`docs/` 目录）始终与最新代码处于同一版本。

当代码变更涉及以下内容时，应在同一 PR 中更新对应文档：
- 新增或移除模块 → 更新 `docs/guide/architecture.md`（依赖图、分层职责表）
- 公共 API 变更 → 更新 `docs/modules/` 下对应模块文档
- 新增示例或工具 → 更新 `docs/examples/` 或 `docs/guide/` 相关页面
- 架构决策变更 → 更新 `docs/guide/architecture.md` ADR 摘要表

VitePress 构建命令（在 gitbook 工作树中执行）：
- `npm run docs:dev` — 本地预览
- `npm run docs:build` — 构建产物
- `npm run docs:preview` — 预览构建产物

#!/usr/bin/env bash
# check-deps.sh — 核心模块依赖方向自动检查（PRD S8 / M0 退出条件）。
#
# 规则（架构文档 03-architecture-overview.md §6）：
#   - agent-model：不依赖任何内部 module
#   - agent-core：仅依赖 agent-model
#   - agent-plugin：仅依赖 agent-model、agent-core
#   - agent-compose：仅依赖 agent-model、agent-core、agent-plugin
#   - adapters/*：仅依赖 agent-model
#   - tools：仅依赖 agent-model、agent-core
#   - cli：仅依赖以上全部（入口层）
#   - 任何 module 不得依赖 agent-runtime（M4 前禁止提前引入）
#
# 用法：scripts/check-deps.sh（在仓库根执行）
set -euo pipefail
cd "$(dirname "$0")/.."

PREFIX="github.com/mantis-layer/mts"
# 每个 module → 允许依赖的内部 module 集合（空格分隔；不含自身）
declare -A ALLOWED=(
  ["$PREFIX/agent-model"]=""
  ["$PREFIX/agent-core"]="$PREFIX/agent-model"
  ["$PREFIX/agent-plugin"]="$PREFIX/agent-model $PREFIX/agent-core"
  ["$PREFIX/agent-compose"]="$PREFIX/agent-model $PREFIX/agent-core $PREFIX/agent-plugin"
  ["$PREFIX/adapters/model-openai"]="$PREFIX/agent-model"
  ["$PREFIX/tools"]="$PREFIX/agent-model $PREFIX/agent-core"
  ["$PREFIX/cli"]="$PREFIX/agent-model $PREFIX/agent-core $PREFIX/agent-plugin $PREFIX/agent-compose $PREFIX/adapters/model-openai $PREFIX/tools"
)

FAIL=0
for mod in "${!ALLOWED[@]}"; do
  deps=$(go list -deps -f '{{.ImportPath}}' "$mod/..." 2>/dev/null \
    | grep "^$PREFIX/" | sort -u || true)
  for d in $deps; do
    # 跳过自身及同 module 内的子包（如 agent-plugin/mcp）
    case "$d" in
      "$mod"|"$mod/"*) continue ;;
    esac
    if ! echo " ${ALLOWED[$mod]} " | grep -q " $d "; then
      echo "违反依赖方向: $mod → 不允许依赖 $d（允许: ${ALLOWED[$mod]:-无}）"
      FAIL=1
    fi
  done
done

if [ $FAIL -eq 0 ]; then
  echo "依赖方向检查通过 ✓"
else
  echo "依赖方向检查失败 ✗"
  exit 1
fi

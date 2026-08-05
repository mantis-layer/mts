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
#   - examples/*：仅依赖以上全部（示例层）
#   - 任何 module 不得依赖 agent-runtime（M4 前禁止提前引入）
#
# 用法：scripts/check-deps.sh（在仓库根执行）
set -euo pipefail
cd "$(dirname "$0")/.."

PREFIX="github.com/mantis-layer/mts"
# 每个 module → 允许依赖的内部 module 前缀集合（空格分隔；不含自身。
# 按前缀匹配：允许依赖 X 意味着允许 X 及其全部子包。）
declare -A ALLOWED=(
  ["$PREFIX/agent-model"]=""
  ["$PREFIX/agent-core"]="$PREFIX/agent-model"
  ["$PREFIX/agent-plugin"]="$PREFIX/agent-model $PREFIX/agent-core"
  ["$PREFIX/agent-compose"]="$PREFIX/agent-model $PREFIX/agent-core $PREFIX/agent-plugin"
  ["$PREFIX/agent-runtime"]="$PREFIX/agent-model $PREFIX/agent-core"
  ["$PREFIX/adapters/model-openai"]="$PREFIX/agent-model"
  ["$PREFIX/tools"]="$PREFIX/agent-model $PREFIX/agent-core"
  ["$PREFIX/cli"]="$PREFIX/agent-model $PREFIX/agent-core $PREFIX/agent-plugin $PREFIX/agent-compose $PREFIX/adapters/model-openai $PREFIX/tools"
  ["$PREFIX/examples/tool_loop_agent"]="$PREFIX/agent-model $PREFIX/agent-core $PREFIX/agent-plugin $PREFIX/agent-compose $PREFIX/adapters/model-openai $PREFIX/tools"
)

# is_allowed 判断依赖 d 是否在允许前缀集合内。
is_allowed() {
  local d="$1" a
  for a in ${ALLOWED[$mod]}; do
    case "$d" in
      "$a"|"$a/"*) return 0 ;;
    esac
  done
  return 1
}

FAIL=0
# 中断兜底清理：进程被信号终止时删除残留临时文件（健壮性）。
TMPFILES=()
trap 'rm -f "${TMPFILES[@]}"' EXIT
for mod in "${!ALLOWED[@]}"; do
  tmpfile=$(mktemp)
  errfile=$(mktemp)
  TMPFILES+=("$tmpfile" "$errfile")
  if ! go list -deps -f '{{.ImportPath}}' "$mod/..." >"$tmpfile" 2>"$errfile"; then
    echo "go list 失败: $mod: $(cat "$errfile")"
    rm -f "$tmpfile" "$errfile"
    FAIL=1
    continue
  fi
  deps=$(grep "^$PREFIX/" "$tmpfile" | sort -u || true)
  rm -f "$tmpfile" "$errfile"
  for d in $deps; do
    # 跳过自身及同 module 内的子包（如 agent-plugin/mcp）
    case "$d" in
      "$mod"|"$mod/"*) continue ;;
    esac
    if ! is_allowed "$d"; then
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

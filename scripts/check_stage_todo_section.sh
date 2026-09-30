#!/usr/bin/env bash
# 阶段"未完成清单"守卫（E2E-23 收尾轮新增）
#
# ── 为什么需要它 ────────────────────────────────────────────────
# E2E-23 收口时，"还剩什么"散落在 report §9.x、§10、账本三处，每轮复核都在口头汇报，
# 但**没有一处是权威的**。2026-09-30 用户直接指出：
# **"先落地文档，太多次了，把要做的写进去，标记为未完成"**。
#
# 于是约定：每个 `status: partial` 的阶段，其 report 必须有 `## 0. 未完成清单` 小节，且
#   ① 声明的条数与实际 `**T-` 条目数一致（防"说 7 条其实 9 条"）
#   ② 每一项都写明**责任人**（用户 / 执行者）与**可核验的完成判据**
#   ③ §10 之类的汇总节只做指针，不维护第二份副本（AP-14）
#
# 守的是治理不是代码质量：这一阶段反复出现的病是
# "完成了但没记 / 记了但散在多处 / 记了但数字不对"，本守卫把它变成可机械检查。
#
# ── 退出码：棘轮（ratchet）────────────────────────────────────
# 历史上有 10 个 partial 阶段早于本约定，自然没有 §0。
# 直接判红会立刻把 CI 弄常红 —— 而常红门禁会训练出"看见红色就跳过"
# （anti-patterns AP-11）。所以：
#   · **有 §0 的阶段**：严格校验，违反即红。
#   · **没有 §0 的阶段**：计入 `legacy`，只报醒目 WARN。
#   · **`legacy` 数不得超过 LEGACY_BASELINE**，否则红 ——
#     即"新增一个 partial 阶段却不写未完成清单"会被抓住；
#     给任一历史阶段补上 §0 后，把基线调小一位。
#   所以 legacy 数只会单调下降。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STAGES_DIR="$REPO_ROOT/docs/e2e-roadmap/stages"

# 允许缺 §0 的历史阶段数量上限。**只能往下调**。
LEGACY_BASELINE=10

fail=0
pass=0
legacy=0

echo "== 阶段未完成清单守卫 =="

if ! command -v python >/dev/null 2>&1; then
  echo "FAIL 环境无 python ⇒ 本项**未验证**（不是通过）"
  exit 1
fi

# tr -d '\r' 兜底：即便解析器实现方式变了，Windows 的 CRLF 也不会渗进路径
partial_dirs="$(python "$REPO_ROOT/scripts/_extract_partial_stages.py" | tr -d '\r')"
if [ -z "$partial_dirs" ]; then
  echo "FATAL: 解析器没解析出任何 partial 阶段（解析器自身故障，非被测对象问题）"
  exit 2
fi

echo "roadmap 标 partial 的阶段共 $(printf '%s' "$partial_dirs" | grep -c .) 个"
echo "缺 §0 的历史阶段上限（LEGACY_BASELINE）: $LEGACY_BASELINE"
echo

for d in $partial_dirs; do
  report="$STAGES_DIR/$d/report.md"
  if [ ! -f "$report" ]; then
    echo "FAIL [$d]: roadmap 标 partial 但找不到 report.md"
    fail=$((fail + 1))
    continue
  fi

  if ! grep -q '^## 0\. 未完成清单' "$report"; then
    legacy=$((legacy + 1))
    echo "WARN [$d]: partial 但无 '## 0. 未完成清单'（历史阶段，计入 legacy $legacy/$LEGACY_BASELINE）"
    continue
  fi
  echo "PASS [$d]: 有 §0 未完成清单"

  declared="$(grep -oE '[0-9]+ 项未完成' "$report" | head -1 | grep -oE '^[0-9]+' || true)"
  actual="$(grep -cE '^\| \*\*T-[0-9]+\*\*' "$report" || true)"

  if [ -z "$declared" ]; then
    echo "FAIL [$d]: §0 未写「N 项未完成」——条数无法被机械核对"
    fail=$((fail + 1))
  elif [ "$declared" != "$actual" ]; then
    echo "FAIL [$d]: §0 声明「$declared 项未完成」，实际 $actual 条 **T-** 条目（数字与事实不符，AP-14）"
    fail=$((fail + 1))
  else
    echo "PASS [$d]: 声明 $declared 项、实际 $actual 条，一致"
    pass=$((pass + 1))
  fi

  bad_row="$(grep -E '^\| \*\*T-[0-9]+\*\*' "$report" | awk -F'|' 'NF < 7 {print $2; exit}')"
  if [ -n "$bad_row" ]; then
    echo "FAIL [$d]: 条目 $bad_row 列数不足 —— 缺责任人或可核验判据"
    fail=$((fail + 1))
  else
    echo "PASS [$d]: 每条都有责任人与可核验判据"
    pass=$((pass + 1))
  fi
done

echo
if [ "$legacy" -gt "$LEGACY_BASELINE" ]; then
  echo "FAIL: 缺 §0 的 partial 阶段有 $legacy 个，超过基线 $LEGACY_BASELINE"
  echo "      ⇒ 新增了 partial 阶段却没写未完成清单，或基线被上调。基线只允许往下调。"
  fail=$((fail + 1))
elif [ "$legacy" -gt 0 ]; then
  echo "WARN: 仍有 $legacy/$LEGACY_BASELINE 个历史 partial 阶段没有未完成清单（债务可见，只降不升）"
fi

echo "PASS: $pass  FAIL: $fail  legacy: $legacy"
if [ "$fail" -gt 0 ]; then
  echo "RED：未完成清单缺失、不一致或字段不全"
  exit 1
fi
echo "GREEN：所有有 §0 的 partial 阶段，清单完整且数字自洽"

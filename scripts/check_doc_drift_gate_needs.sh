#!/usr/bin/env bash
# doc-drift-gate 的 needs 覆盖守卫（E2E-23 收尾轮）
#
# ── 为什么需要它 ────────────────────────────────────────────────
# 分支保护里 23 条 required status checks 全是**精确 job 名**（含中文）。这带来两个问题：
#   ① 列表长到 33 条，维护成本高；
#   ② **谁改了 job 的 name:，门禁会静默失效** —— 精确匹配不上，GitHub 不报错，
#      那条检查从此不再被拦，而门禁设置页看起来一切正常（正是 AP-11 的形态）。
#
# 解法是加一个汇总 job `doc-drift-gate`：它 needs 全部 23 个检查，
# 任一失败就失败。分支保护只需填 1 条，且**新增 job 时只要被 gate 覆盖就自动纳入**。
#
# 但这引入了新风险：**gate 的 needs 是硬编码列表**。
# 有人新增一个检查 job 却忘了加进 needs ⇒ 那个检查**永远不进 gate** ⇒ 门禁看起来生效、实际漏检。
# 这正是本脚本要拦的：**没有机器校验，needs 列表迟早会漂。**
#
# 纪律：本脚本必须**独立于 gate 单独跑**（不 needs gate、不被 gate 覆盖），
# 否则 gate 自己漏掉自己就没人发现了。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WF="$REPO_ROOT/.github/workflows/doc-drift-check.yml"
GATE_JOB="doc-drift-gate"
SELF_JOB="doc-drift-needs-sync"

echo "== doc-drift-gate 的 needs 覆盖守卫 =="

if [ ! -f "$WF" ]; then
  echo "FAIL: 找不到 $WF"
  exit 2
fi

# 1) workflow 里定义的全部 job key（`jobs:` 之后、二级缩进的 `key:` 行）
all_jobs="$(awk '
  /^jobs:[[:space:]]*$/ { in_jobs = 1; next }
  in_jobs && /^[^[:space:]]/ { in_jobs = 0 }
  in_jobs && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ {
    k = $1; sub(/:$/, "", k); print k
  }
' "$WF" | sort)"

# 2) gate 的 needs 列表
gate_needs="$(awk -v gate="$GATE_JOB" '
  $0 ~ "^  " gate ":[[:space:]]*$" { in_gate = 1; next }
  in_gate && /^  [A-Za-z0-9_-]+:[[:space:]]*$/ { in_gate = 0 }
  in_gate && /^    needs:/ {
    line = $0
    sub(/^    needs:[[:space:]]*/, "", line)
    gsub(/^\[|\]$/, "", line)
    n = split(line, parts, ",")
    for (i = 1; i <= n; i++) {
      p = parts[i]
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", p)
      gsub(/^-|:/, "", p)
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", p)
      if (p != "") print p
    }
    exit
  }
' "$WF" | sort -u)"

if [ -z "$gate_needs" ]; then
  echo "FAIL: 在 $WF 里找不到 job '$GATE_JOB' 的 needs 列表"
  echo "      ⇒ 分支保护若只填 gate 一条，等于**一个检查都不拦**"
  exit 1
fi

check_jobs="$(printf '%s\n' "$all_jobs" | grep -v -x -e "$GATE_JOB" -e "$SELF_JOB")"
check_count="$(printf '%s' "$check_jobs" | grep -c . )"
need_count="$(printf '%s' "$gate_needs" | grep -c . )"

echo "  workflow 内 job 总数: $(printf '%s' "$all_jobs" | grep -c . )（其中检查 job $check_count 个）"
echo "  gate 的 needs 条数: $need_count"

fail=0
# 3) 每个检查 job 都必须出现在 needs 里
missing=""
for j in $check_jobs; do
  if ! printf '%s\n' "$gate_needs" | grep -qx "$j"; then
    missing="${missing} $j"
  fi
done
if [ -n "$missing" ]; then
  echo "FAIL: 这些 job 不在 '$GATE_JOB' 的 needs 里 ⇒ 它们失败也不会让 gate 失败（门禁看着生效、实际漏检）："
  for m in $missing; do echo "       - $m"; done
  fail=$((fail + 1))
fi

# 4) needs 里不能有已不存在的 job（删了 job 却没清 needs ⇒ 静默失效的另一面）
stale=""
for j in $gate_needs; do
  if ! printf '%s\n' "$all_jobs" | grep -qx "$j"; then
    stale="${stale} $j"
  fi
done
if [ -n "$stale" ]; then
  echo "FAIL: gate 的 needs 里引用了 workflow 中已不存在的 job（GitHub 会直接报 workflow 无效）："
  for s in $stale; do echo "       - $s"; done
  fail=$((fail + 1))
fi

# 5) 反向健全性：不能出现"needs 里有、但一个检查 job 都没有"的空壳门禁
if [ "$need_count" -lt 20 ]; then
  echo "FAIL: gate 只 needs 了 $need_count 项，疑似被误改成空壳（当前应有 $check_count 个检查 job）"
  fail=$((fail + 1))
fi

echo
if [ "$fail" -gt 0 ]; then
  echo "RED：汇总门禁的 needs 覆盖存在缺口（$fail 项）"
  exit 1
fi

echo "GREEN：gate 覆盖全部 $check_count 个检查 job，且无失效引用"

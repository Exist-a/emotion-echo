#!/usr/bin/env bash
# E2E-25 M2（D-36 决议，用户裁定 B+）回归守卫：
# dev-up.sh 起环境完成后必须跑 `check_apisix_drift.sh extras`，
# 把"seed 白名单外的野生路由"（route 116/299 类漂移）在每次启动时打出来。
#
# 语义：**只报不拦**（B+ 覆盖+报告）——检查失败（exit 1）不阻断 dev-up，
# 但必须打印告警行；静默跳过 = 漂移重新不可见，本守卫判红。
#
# 运行：bash scripts/test_devup_drift_check.sh
# 负向对照：从 dev-up.sh 删掉 check_apisix_drift 调用 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEVUP="$REPO_ROOT/scripts/dev-up.sh"
DRIFT="$REPO_ROOT/scripts/check_apisix_drift.sh"

fail=0
pass=0

echo "== E2E-25 M2 dev-up 漂移检查守卫 =="

for f in "$DEVUP" "$DRIFT"; do
  if [ ! -f "$f" ]; then
    echo "FATAL: 找不到 $f"
    exit 2
  fi
done
pass=$((pass + 1))
echo "PASS dev-up.sh 与 check_apisix_drift.sh 均存在"

# 1) dev-up.sh 必须调用 drift extras（B+：每次启动自动报告）
if grep -q "check_apisix_drift.sh" "$DEVUP" && grep -q "extras" "$DEVUP"; then
  echo "PASS dev-up.sh 调用 check_apisix_drift.sh extras（B+ 接入）"
  pass=$((pass + 1))
else
  echo "FAIL dev-up.sh 未调用 drift extras —— 漂移在启动流程中不可见（违反 D-36 B+）"
  fail=$((fail + 1))
fi

# 2) 必须是"只报不拦"：检查失败（extras 返 1）不得因 set -e 中断启动。
#    合法形态：`... extras || true` 或 `if ! ... extras; then <告警> fi`。
drift_cmd_line=$(grep -n "check_apisix_drift.sh.*extras" "$DEVUP" | head -1 || true)
if echo "$drift_cmd_line" | grep -q "|| true"; then
  echo "PASS extras 调用带 || true（只报不拦，B+ 语义）"
  pass=$((pass + 1))
else
  # if ! 形态：同一行（或紧邻上下文）须有 if ! 且后续有 fi 关闭
  ctx="$(grep -n -B2 -A6 "check_apisix_drift.sh.*extras" "$DEVUP" || true)"
  if echo "$ctx" | grep -q "if !" && echo "$ctx" | grep -q "fi"; then
    echo "PASS extras 在 if ! ... fi 块中（只报不拦，B+ 语义）"
    pass=$((pass + 1))
  else
    echo "FAIL extras 调用既无 || true 也无 if ! 保护 —— set -e 会中断启动（误成 fail-closed）"
    fail=$((fail + 1))
  fi
fi

# 3) 调用位置必须在 seed 之后（drift 对比对象是 seed 的产物）
#    dev-up.sh 里 compose up apisix-seed / seed 相关行应出现在 extras 行之前。
seed_line=$(grep -n "apisix-seed" "$DEVUP" | head -1 | cut -d: -f1 || true)
drift_line=$(grep -n "check_apisix_drift.sh" "$DEVUP" | head -1 | cut -d: -f1 || true)
if [ -n "$seed_line" ] && [ -n "$drift_line" ] && [ "$drift_line" -gt "$seed_line" ]; then
  echo "PASS extras 调用位于 apisix-seed 之后（行 $seed_line < $drift_line）"
  pass=$((pass + 1))
else
  echo "FAIL extras 调用缺失或位于 apisix-seed 之前（seed_line=$seed_line drift_line=$drift_line）"
  fail=$((fail + 1))
fi

# 4) (E2E-26 F-181) 路径必须可解析：dev-up 开头 `cd .../deploy` 之后，
#    `$(dirname "$0")/check_apisix_drift.sh`（$0 为相对调用时）解析到
#    deploy/scripts/... 不存在 ⇒ bash 返 127 ⇒ 被 if ! 当成"存在漂移"假告警，
#    真漂移永远查不到（2026-10-03 开工实测复现）。
drift_invoke_line=$(grep -n 'bash .*check_apisix_drift\.sh.*extras' "$DEVUP" | head -1 || true)
if echo "$drift_invoke_line" | grep -q '\$(dirname "\$0")/check_apisix_drift.sh'; then
  echo "FAIL (F-181): drift 调用依赖 cd 之后的相对 dirname \$0 —— 路径不存在（实测 No such file → 127 → 假漂移告警）"
  fail=$((fail + 1))
else
  echo "PASS drift 调用不依赖 cd 后的相对 dirname \$0（F-181）"
  pass=$((pass + 1))
fi

# 5) (F-181) 工具缺失必须显式报「检查未执行」——不得把退出码 127 误报成漂移。
if grep -qE '检查未执行|工具缺失' "$DEVUP"; then
  echo "PASS 工具缺失分支显式区分「未执行」与「有漂移」（F-181）"
  pass=$((pass + 1))
else
  echo "FAIL (F-181): 无「工具缺失≠漂移」分支 —— 127 仍触发假漂移告警"
  fail=$((fail + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"
if [ "$fail" -gt 0 ]; then
  echo "RED：dev-up 未按 D-36 B+ 接入漂移报告"
  exit 1
fi
echo "GREEN：dev-up 漂移报告接入且只报不拦"

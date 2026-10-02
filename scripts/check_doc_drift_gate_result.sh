#!/usr/bin/env bash
# E2E-F-173：文档守卫总闸的结果聚合判定（stdin 读 GitHub Actions needs JSON）。
#
# 判定语义（契约，由 scripts/test_check_doc_drift_gate_result.sh 钉死）：
#   success  → 通过
#   skipped  → 通过（adr-gate 在 push 事件下被 `if: pull_request` 有意 skip，
#               是 E2E-F-127 的设计——PR 上已过门禁；把 skipped 判红会让
#               main 的 push 总闸恒红，稀释"红 = 有问题"信号，属 AP-11）
#   failure / cancelled / 其它 → 失败（报出 job 名与 result）
#
# 用法：echo "$NEEDS_JSON" | bash scripts/check_doc_drift_gate_result.sh
# 退出码：0 全过；1 存在非 success/skipped 项；2 解释器不可用。
set -uo pipefail

# 解释器必须**实跑验证**再用（windows-python3-store-stub 教训）：
# 本机 python3 是 Store 别名桩（command -v 找得到但静默不执行 rc=49），
# CI ubuntu 上则可能只有 python3。逐个探测"能打印 1 的"才算真解释器。
PY=""
for cand in python python3; do
  if out=$("$cand" -c 'print(1)' 2>/dev/null) && [ "$out" = "1" ]; then
    PY="$cand"
    break
  fi
done
if [ -z "$PY" ]; then
  echo "::error::无可用 python 解释器（python/python3 均为桩或缺失）"
  exit 2
fi

"$PY" -c '
import json, sys
try:
    d = json.load(sys.stdin)
except Exception as e:
    print("::error::总闸输入不是合法 JSON: %s" % e)
    sys.exit(1)
failed = [(k, v.get("result")) for k, v in d.items()
          if v.get("result") not in ("success", "skipped")]
if not failed:
    skipped = sum(1 for v in d.values() if v.get("result") == "skipped")
    print("OK: %d 项文档守卫全部通过（其中 skipped %d 项按 E2E-F-173 视为通过）" % (len(d), skipped))
    sys.exit(0)
for k, r in failed:
    print("::error::检查未通过: %s (%s)" % (k, r))
sys.exit(1)
'

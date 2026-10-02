#!/usr/bin/env bash
# E2E-F-173 回归守卫：总闸聚合判定必须把 skipped 视为通过（adr-gate 在
# push 事件下被 if: pull_request 有意 skip——E2E-F-127 设计，PR 已过门禁）。
#
# 背景：旧内联 python 判 `result != "success"` 即失败 ⇒ main 的 push 事件上
# 总闸恒红（skipped 被判 fail），"红 = 有问题"的信号被稀释（anti-pattern AP-11）。
#
# 修法：聚合逻辑抽成本脚本测的对象 scripts/check_doc_drift_gate_result.sh
# （stdin 读 needs JSON），workflow 调它。本测试喂三种形态验证判定语义。
#
# 运行：bash scripts/test_check_doc_drift_gate_result.sh

set -uo pipefail
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECKER="$REPO_ROOT/scripts/check_doc_drift_gate_result.sh"

fail=0
pass=0
echo "== E2E-F-173 总闸聚合判定守卫 =="

if [ ! -f "$CHECKER" ]; then
  echo "FATAL: 找不到 $CHECKER"
  exit 2
fi
pass=$((pass + 1))
echo "PASS check_doc_drift_gate_result.sh 存在"

run() { printf '%s' "$1" | bash "$CHECKER"; }

# 1) 全 success → 0
if out=$(run '{"a":{"result":"success"},"b":{"result":"success"}}'); then
  echo "PASS 全 success → exit 0（输出：$out）"
  pass=$((pass + 1))
else
  echo "FAIL 全 success 应 exit 0（rc=$? out=$out）"
  fail=$((fail + 1))
fi

# 2) success + skipped → 0（本守卫核心：push 事件 adr-gate skip 不得判红）
if out=$(run '{"adr-gate":{"result":"skipped"},"others":{"result":"success"}}'); then
  echo "PASS success+skipped → exit 0（skipped 不判红）"
  pass=$((pass + 1))
else
  echo "FAIL success+skipped 应 exit 0（F-173 回归——rc=$? out=$out）"
  fail=$((fail + 1))
fi

# 3) 含 failure → 非 0 且报出行名
if out=$(run '{"adr-gate":{"result":"skipped"},"tdd":{"result":"failure"}}'); then
  echo "FAIL 含 failure 应非零退出"
  fail=$((fail + 1))
else
  echo "$out" | grep -q "tdd" && grep -q "failure" <<<"$out" && {
    echo "PASS 含 failure → 非零退出且报出 tdd/failure"
    pass=$((pass + 1))
  } || {
    echo "FAIL 退出码对但未报告失败项（$out）"
    fail=$((fail + 1))
  }
fi

# 4) cancelled 也必须算失败（不属 skipped）
if out=$(run '{"a":{"result":"cancelled"}}'); then
  echo "FAIL cancelled 应非零退出"
  fail=$((fail + 1))
else
  echo "PASS cancelled → 非零退出"
  pass=$((pass + 1))
fi

echo
echo "PASS: $pass  FAIL: $fail"
if [ "$fail" -gt 0 ]; then
  echo "RED：聚合判定语义不符（E2E-F-173）"
  exit 1
fi
echo "GREEN：success/skipped=通过、failure/cancelled=失败"

#!/usr/bin/env bash
# scripts/check_dead_grpc_inventory.sh — E2E-31 T-1「已知未接线 RPC 清单」守卫
#
# 背景（账本 E2E-F-208 / stages/e2e-31-internal-rpc-convergence/report.md §0 T-1）：
#   全仓有 7 个内部 gRPC RPC 处于「已知未接线」状态。用户 2026-10-09 裁定
#   **保留 + 标注 + 守卫**（不删、也不在这个阶段接线）——因为经查它们**不是**"业务走了
#   HTTP 把 gRPC 绕开"（那是 assessment 链的情况，已修），而是"功能未接出"或"族内冗余"：
#     服务端 7 处：chat StreamMessages / llm AnalyzeBatch / analytics MentalHealth{History,Trigger,Trend}
#                  / user {Logout, VerifySecurityAnswer}
#     BFF 客户端 3 处：chat_grpc.StreamMessages / user_grpc.{Logout, VerifySecurityAnswer}
#
# 本守卫保证「清单不失真」：
#   · 若有人把某 RPC 接上线（好事）⇒ 他会删掉该处标记 ⇒ 总数 -1 ⇒ **本守卫 RED**，
#     强制其同步更新本脚本 EXPECTED 与账本 E2E-F-208 —— 即"接线必须是一次显式决策"。
#   · 若标记被误删 / 新加未接线处漏标 ⇒ 同样 RED。
#
# 为什么**不**做"断言零调用方"：这些 RPC 一旦真被接上线，那正是期望结果；把它们钉成
# "永不可调用"是反向约束。本守卫守的是**记录的真实性**，不是禁止使用。

set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

MARKER="E2E-31 已知未接线"
# 7 个 RPC：服务端 7 处 + BFF 客户端 3 处 = 10（见上表）
EXPECTED=10

echo "== 未接线清单守卫（E2E-31 T-1）=="

hits="$(grep -rn "$MARKER" --include='*.go' --include='*.py' --include='*.sh' . 2>/dev/null \
  | grep -v '/node_modules/' \
  | grep -v 'check_dead_grpc_inventory.sh' || true)"

if [ -z "$hits" ]; then
  actual=0
else
  actual="$(printf '%s\n' "$hits" | wc -l | tr -d ' ')"
fi

printf '%s\n' "$hits" | sed 's/:[0-9]*:.*//' | sort | uniq -c | sed 's/^/   /'

if [ "$actual" != "$EXPECTED" ]; then
  echo "FAIL: 标记数 $actual ≠ 预期 $EXPECTED"
  echo "      ⇒ 要么有 RPC 被接线（请删除对应标记，并同步本脚本 EXPECTED 与账本 E2E-F-208），"
  echo "        要么有新未接线处漏标 / 标记被误删。两种都必须是一次显式决策，不得静默变更。"
  exit 1
fi

echo "PASS: 未接线标记 $actual/$EXPECTED，清单与代码一致"
exit 0

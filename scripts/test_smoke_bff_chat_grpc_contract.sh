#!/usr/bin/env bash
# scripts/test_smoke_bff_chat_grpc_contract.sh — smoke_bff_chat_grpc.sh 的静态守卫
#
# 为什么需要（E2E-29 F-182 的教训）：该 smoke 自 jwt-auth 落地起**恒 4/7 FAIL**——
# 契约 4~7 只带 `X-User-Id` 不带令牌，被网关 401。它坏了很久没人发现，因为脚本没被
# 任何门禁看着。本守卫把"必须带令牌"钉成静态契约，并含负向自检（证明有牙齿）。
#
# 契约：
#   1  脚本含登录取 token 步骤（POST /api/v1/auth/login）
#   2  受保护端点调用带 `Authorization: Bearer $TOKEN`（≥4 处）
#   3  脚本**不得**再出现 `-H "X-User-Id: $USER_ID"`（回到无令牌调用即假绿）
#   4  create 响应 id 解析兼容字符串 id（"id":"551"）
#
# 退出码：0 全过 / 1 任一不过
# 用法：bash scripts/test_smoke_bff_chat_grpc_contract.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SMOKE="$SCRIPT_DIR/smoke_bff_chat_grpc.sh"

pass=0
fail=0
ok()  { echo "  [PASS] $*"; pass=$((pass + 1)); }
bad() { echo "  [FAIL] $*" >&2; fail=$((fail + 1)); }

check_file() { # $1=path
  local f="$1"
  if grep -q '/api/v1/auth/login' "$f" && grep -q 'TOKEN=' "$f"; then
    ok "契约 1：含登录取 token 步骤"
  else
    bad "契约 1：缺登录取 token 步骤（F-182 回归）"
  fi
  local n
  n=$(grep -c 'Authorization: Bearer \$TOKEN' "$f" || true)
  if [ "$n" -ge 4 ]; then
    ok "契约 2：受保护端点带 Bearer 令牌（$n 处）"
  else
    bad "契约 2：带令牌的调用只有 $n 处（期望 ≥4）"
  fi
  if grep -q 'X-User-Id: \$USER_ID' "$f"; then
    bad "契约 3：脚本仍以 X-User-Id 直接调用受保护端点（无令牌 ⇒ 恒 401 假绿）"
  else
    ok "契约 3：无裸 X-User-Id 调用"
  fi
  if grep -q '"id"\[\[:space:\]\]\*:\[\[:space:\]\]\*"?' "$f"; then
    ok "契约 4：create 响应 id 解析兼容字符串 id"
  else
    bad "契约 4：id 解析未兼容字符串形态（BFF 自 Stage 72 起返回 \"id\":\"551\"）"
  fi
}

echo "=== smoke_bff_chat_grpc 静态守卫（E2E-29 F-182）==="
check_file "$SMOKE"

# ---- 负向自检：把 Bearer 换回 X-User-Id，契约 2/3 必须转红 ----
echo "--- 负向自检（副本回退成 X-User-Id 调用）---"
TMP="$(mktemp -d 2>/dev/null || echo "${TEMP:-/tmp}/smoke-guard-$$")"
mkdir -p "$TMP"
trap 'rm -rf "$TMP"' EXIT
python - "$SMOKE" "$TMP/neg.sh" <<'PYEOF'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src, encoding="utf-8").read()
t = t.replace('-H "Authorization: Bearer $TOKEN"', '-H "X-User-Id: $USER_ID"')
open(dst, "w", encoding="utf-8").write(t)
PYEOF
neg_bad=0
if grep -q 'Authorization: Bearer \$TOKEN' "$TMP/neg.sh"; then neg_bad=$((neg_bad+1)); fi
if ! grep -q 'X-User-Id: \$USER_ID' "$TMP/neg.sh"; then neg_bad=$((neg_bad+1)); fi
if [ "$neg_bad" -eq 0 ]; then
  ok "负向对照：回退后契约 2/3 判定会转红（守卫有牙齿）"
else
  bad "负向对照失效（副本未被改造，$neg_bad 项）"
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ] || exit 1
exit 0

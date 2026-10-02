#!/usr/bin/env bash
# E2E-25 C2：check_apisix_drift.sh 的离线契约测试（无网关依赖）
#
# 覆盖：bash 语法 / diff 模式（fixture 快照）/ extras-from 模式（fixture 路由列表）/
#       退出码契约。网络相关模式（snapshot/verify）需真实网关，属运行时验收。
# 运行：bash scripts/test_check_apisix_drift.sh

set -u
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOOL="$SCRIPT_DIR/check_apisix_drift.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() { echo "  ✗ $*" >&2; exit 1; }
pass() { echo "  ✓ $*"; }

[ -f "$TOOL" ] || fail "check_apisix_drift.sh not found"
pass "check_apisix_drift.sh exists"

bash -n "$TOOL" || fail "bash -n failed"
pass "bash -n syntax OK"

# ---- diff 模式（fixture）----
cat > "$TMP/before.txt" <<'EOF'
consumers/emotion_echo_bff {...}
routes/100 {"uri":"/api/v1/*","status":1}
routes/110 {"uri":"/api/v1/auth/login","status":1}
upstreams/1 {"name":"user-svc"}
EOF
cat > "$TMP/after_tamper.txt" <<'EOF'
consumers/emotion_echo_bff {...}
routes/100 {"uri":"/api/v1/*","status":0}
routes/110 {"uri":"/api/v1/auth/login","status":1}
routes/299 {"uri":"/api/v1/__drift_probe__"}
upstreams/1 {"name":"user-svc"}
EOF

if out=$("$TOOL" diff "$TMP/before.txt" "$TMP/before.txt" 2>&1); then
  pass "diff 无差异 → exit 0"
else
  fail "diff 无差异应 exit 0（实际非零）：$out"
fi
echo "$out" | grep -q "no drift" || fail "无差异输出应含 no drift"

if out=$("$TOOL" diff "$TMP/before.txt" "$TMP/after_tamper.txt" 2>&1); then
  fail "diff 有差异应非零退出（篡改+新增同时存在）"
else
  rc=$?
  pass "diff 有差异 → 非零退出（rc=$rc）"
fi
echo "$out" | grep -q "routes/100" || fail "报告应指出 routes/100 被篡改"
echo "$out" | grep -q "routes/299" || fail "报告应指出 routes/299 新增"

# ---- extras-from 模式（fixture：白名单来自 seed.sh 真实解析）----
cat > "$TMP/routes.json" <<'EOF'
{"value":[{"id":"100"},{"id":"205"},{"id":"299"},{"id":"116"}]}
EOF
if out=$("$TOOL" extras-from "$TMP/routes.json" 2>&1); then
  fail "extras 存在（299）应非零退出"
else
  pass "extras-from 白名单外路由 → 非零退出"
fi
echo "$out" | grep -q "^299$" || fail "输出应含 299 一行"
echo "$out" | grep -q "^116$" && fail "116 是漂移清理目标，不得算 extras" || pass "116（漂移清理目标）未误报"

cat > "$TMP/clean.json" <<'EOF'
{"value":[{"id":"100"},{"id":"205"},{"id":"110"}]}
EOF
if out=$("$TOOL" extras-from "$TMP/clean.json" 2>&1); then
  pass "extras-from 白名单内 → exit 0"
else
  fail "白名单内应 exit 0：$out"
fi

echo ""
echo "Summary: PASS"

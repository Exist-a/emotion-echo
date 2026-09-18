#!/usr/bin/env bash
# scripts/test_check_secrets.sh —— check_secrets.sh 的负向/正向测试
#
# 为什么需要（R-03 #7 / AP-13）：一个"永远退出 0"的扫描器也能通过"在干净仓库里跑绿"的验证。
# 必须证明它在**有密钥的输入**下会红、在**占位符/注释**下不会误报。
#
# 用例：① 已知泄露字面量 → RED；② 公开令牌格式 → RED；
#       ③ 密钥语义变量赋长字面量 → RED；④ 占位符 → GREEN；⑤ 注释提及历史值 → GREEN。
#
# 用法：bash scripts/test_check_secrets.sh   退出码 0=全过

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCANNER="$SCRIPT_DIR/check_secrets.sh"
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
pass=0; fail=0

run_case() {
  local name="$1" want="$2" f="$3"
  local out rc
  out="$(SECRET_SCAN_TARGETS="$f" bash "$SCANNER" 2>&1)"; rc=$?
  if [ "$rc" = "$want" ]; then
    echo "  [PASS] $name（exit=$rc，期望 $want）"; pass=$((pass+1))
  else
    echo "  [FAIL] $name：exit=$rc，期望 $want"; echo "$out" | sed 's/^/         /' | head -6; fail=$((fail+1))
  fi
}

echo "=== check_secrets.sh 负向/正向测试 ==="

# ① 已知泄露字面量（值在扫描器的 KNOWN_LEAKED 里；此处用拼接避免本测试文件自身被扫中）
LEAKED="WhZEPlrGviCSXlKF""fALZlQWinluoGAbj"
printf 'APISIX_ADMIN_KEY=%s\n' "$LEAKED" > "$TMP/leaked.yml"
run_case "① 已知泄露字面量应 RED" 1 "$TMP/leaked.yml"

# ② 公开令牌格式（GitHub PAT 样例，非真实 token）
printf 'token: ghp_%s\n' "0123456789abcdefghijklmnopqrst" > "$TMP/token.yml"
run_case "② 公开令牌格式应 RED" 1 "$TMP/token.yml"

# ③ 密钥语义变量赋长字面量
printf 'BFF_JWT_SECRET: %s\n' "9f8a7b6c5d4e3f2a1b0c9d8e7f6a5b4c3d2e1f0a" > "$TMP/assign.yml"
run_case "③ 密钥语义变量赋长字面量应 RED" 1 "$TMP/assign.yml"

# ④ 占位符（豁免规则）
printf 'APISIX_ADMIN_KEY: dev-admin-key-local-only\nBFF_JWT_SECRET: ${BFF_JWT_SECRET:-dev-jwt-secret-local-only}\n' > "$TMP/placeholder.yml"
run_case "④ 占位符/env 兜底应 GREEN" 0 "$TMP/placeholder.yml"

# ⑤ 注释里提及历史值（历史说明允许）
{ printf '# 历史：曾用 %s 作为默认值，已轮换\n' "$LEAKED"; printf 'APISIX_ADMIN_KEY: dev-admin-key-local-only\n'; } > "$TMP/comment.yml"
run_case "⑤ 注释提及历史值应 GREEN" 0 "$TMP/comment.yml"

echo ""
echo "==========="; echo "PASS=$pass FAIL=$fail"; echo "==========="
[ "$fail" -gt 0 ] && exit 1
exit 0

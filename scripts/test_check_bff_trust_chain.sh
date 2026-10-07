#!/usr/bin/env bash
# scripts/test_check_bff_trust_chain.sh — check_bff_trust_chain.sh 的自检（负向对照）
#
# 为什么需要：守卫"能 PASS"不代表"有牙齿"。本脚本用**临时副本**注入两类真实违规，
# 断言守卫必须转红（否则守卫只是装饰，AP-11 变体）。
#
# 用例：
#   正向  真实仓库 → 守卫 exit 0
#   负向1 副本把 `- "8894:8894"` 加回 compose.dev.yml → 守卫必须 exit != 0
#   负向2 副本删掉 main.go 里的 ValidateAuthTrust 调用 → 守卫必须 exit != 0
#
# 退出码：0 全过 / 1 任一不过
# 用法：bash scripts/test_check_bff_trust_chain.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GUARD="$SCRIPT_DIR/check_bff_trust_chain.sh"

pass=0
fail=0
ok()  { echo "  [PASS] $*"; pass=$((pass + 1)); }
bad() { echo "  [FAIL] $*" >&2; fail=$((fail + 1)); }

TMP="$(mktemp -d 2>/dev/null || echo "${TEMP:-/tmp}/bff-trust-selftest-$$")"
mkdir -p "$TMP"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

echo "=== check_bff_trust_chain 自检 ==="

# ---- 正向：真实仓库必须 PASS ----
echo "--- 用例 1/3 正向（真实仓库应 PASS）---"
if REPO_ROOT="$REPO_ROOT" bash "$GUARD" >/dev/null 2>&1; then
  ok "真实仓库守卫 PASS"
else
  bad "真实仓库守卫 FAIL（当前仓库已违反契约）"
fi

# ---- 负向 1：把 8894 发布加回 dev overlay ----
echo "--- 用例 2/3 负向（副本加回 8894 发布，守卫必须转红）---"
mkdir -p "$TMP/neg1/deploy" "$TMP/neg1/emotion-echo-web-bff/internal/config"
cp "$REPO_ROOT/deploy/docker-compose.apps.yml" "$TMP/neg1/deploy/"
cp "$REPO_ROOT/emotion-echo-web-bff/main.go" "$TMP/neg1/emotion-echo-web-bff/"
cp "$REPO_ROOT/emotion-echo-web-bff/internal/config/config.go" "$TMP/neg1/emotion-echo-web-bff/internal/config/"
python - "$REPO_ROOT/deploy/compose.dev.yml" "$TMP/neg1/deploy/compose.dev.yml" <<'PYEOF'
import sys
src, dst = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()
marker = "  emotion-echo-web-bff:\n"
inject = "    ports:\n      - \"8894:8894\"\n"
assert marker in text, "compose.dev.yml 里找不到 web-bff 块锚点"
text = text.replace(marker, marker + inject, 1)
open(dst, "w", encoding="utf-8").write(text)
PYEOF
cp "$REPO_ROOT/deploy/compose.prod.yml" "$TMP/neg1/deploy/"
if REPO_ROOT="$TMP/neg1" bash "$GUARD" >/dev/null 2>&1; then
  bad "注入 8894 发布后守卫仍 PASS —— 契约 2 无牙齿"
else
  ok "注入 8894 发布后守卫转红（契约 2 有效）"
fi

# ---- 负向 2：删掉 main.go 的校验调用 ----
echo "--- 用例 3/3 负向（副本删掉 main 的校验调用，守卫必须转红）---"
cp -r "$TMP/neg1" "$TMP/neg2"
python - "$REPO_ROOT/emotion-echo-web-bff/main.go" "$TMP/neg2/emotion-echo-web-bff/main.go" <<'PYEOF'
import sys
src, dst = sys.argv[1], sys.argv[2]
text = open(src, encoding="utf-8").read()
# 复原 compose.dev.yml 的违规注入（本用例只验契约 3）
text = text.replace("c.ValidateAuthTrust()", "/* removed by selftest */ nil")
open(dst, "w", encoding="utf-8").write(text)
PYEOF
cp "$REPO_ROOT/deploy/compose.dev.yml" "$TMP/neg2/deploy/compose.dev.yml"
if REPO_ROOT="$TMP/neg2" bash "$GUARD" >/dev/null 2>&1; then
  bad "删掉 main 校验调用后守卫仍 PASS —— 契约 3 无牙齿"
else
  ok "删掉 main 校验调用后守卫转红（契约 3 有效）"
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ] || exit 1
exit 0

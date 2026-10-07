#!/usr/bin/env bash
# scripts/check_bff_trust_chain.sh — BFF 信任链契约（E2E-29 D-47，2026-10-07 用户拍板）
#
# 背景（两个都真实发生过）：
#   ① 账本 E2E-F-202：BFF 的 8894 宿主映射 + 宽 CIDR ⇒ 直连发 `X-User-Id: 2` 即得
#      smoke_user 数据（冒充任意用户，绕过 APISIX 验签与注入）。
#   ② Stage 109a：`BFF_TRUST_APISIX=true` + `CIDRS` 为空 ⇒ 中间件对所有来源 fail-closed
#      ⇒ 全站 401（排查成本极高的线上形态）。
#
# 本守卫是**静态**的（不依赖 docker / 运行栈），钉住四件事：
#   契约 1  apps.yml 的 web-bff 块不得发布 8894 宿主端口
#   契约 2  compose.dev.yml 的 web-bff 块不得发布 8894（dev 也不留直连面）
#   契约 3  BFF 侧 fail-fast 校验存在**且被 main 调用**（防"写了没接线"）
#   契约 4  prod overlay 仍保留 TrustAPISIX 的 prod 要求（防线索被删）
#
# 退出码：0 = 全 PASS / 1 = 至少 1 项 FAIL
# 用法：bash scripts/check_bff_trust_chain.sh   （REPO_ROOT 可覆盖，供负向自检用副本）

set -uo pipefail

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"

pass=0
fail=0
ok()  { echo "  [PASS] $*"; pass=$((pass + 1)); }
bad() { echo "  [FAIL] $*" >&2; fail=$((fail + 1)); }

echo "=== BFF 信任链契约（E2E-29 D-47）==="
echo "REPO_ROOT=$REPO_ROOT"
echo

# 提取某个 compose 文件的指定 service 块（按缩进边界）。
extract_service_block() {
  python - "$1" "$2" <<'PYEOF'
import re, sys
path, svc = sys.argv[1], sys.argv[2]
try:
    text = open(path, encoding="utf-8").read()
except OSError:
    print("")
    sys.exit(0)
out, inside = [], False
for line in text.split("\n"):
    if re.match(rf"^  {re.escape(svc)}:\s*$", line):
        inside = True
        out.append(line)
        continue
    if inside:
        if re.match(r"^  [A-Za-z0-9_-]+:\s*$", line):
            break
        out.append(line)
print("\n".join(out))
PYEOF
}

# ---------- 契约 1/2：不得发布 8894 ----------
for pair in "docker-compose.apps.yml:契约 1 apps.yml" "compose.dev.yml:契约 2 compose.dev.yml"; do
  f="${pair%%:*}"; label="${pair##*:}"
  block="$(extract_service_block "$REPO_ROOT/deploy/$f" emotion-echo-web-bff)"
  if [ -z "$block" ]; then
    bad "$label：在 deploy/$f 中找不到 emotion-echo-web-bff 服务块"
    continue
  fi
  if printf '%s\n' "$block" | grep -qE '^\s*-\s*"8894:'; then
    bad "$label：deploy/$f 仍发布 8894 宿主端口（直连伪造面，E2E-F-202）"
  else
    ok "$label：deploy/$f 未发布 8894"
  fi
done

# ---------- 契约 3：fail-fast 校验存在且被调用 ----------
cfg="$REPO_ROOT/emotion-echo-web-bff/internal/config/config.go"
main="$REPO_ROOT/emotion-echo-web-bff/main.go"
if grep -q "func (c \*Config) ValidateAuthTrust() error" "$cfg" 2>/dev/null; then
  ok "契约 3a：config.ValidateAuthTrust 已定义"
else
  bad "契约 3a：config.go 缺 ValidateAuthTrust 定义"
fi
if grep -q "c.ValidateAuthTrust()" "$main" 2>/dev/null; then
  ok "契约 3b：main.go 调用了信任链校验（写了没接线 = AP-09 型缺陷）"
else
  bad "契约 3b：main.go 未调用 ValidateAuthTrust（校验形同虚设）"
fi

# ---------- 契约 4：prod overlay 保留 TrustAPISIX 要求 ----------
prod="$REPO_ROOT/deploy/compose.prod.yml"
if grep -q "BFF_TRUST_APISIX" "$prod" 2>/dev/null && grep -q "拒绝启动" "$prod" 2>/dev/null; then
  ok "契约 4：compose.prod.yml 保留 BFF_TRUST_APISIX 的 prod 要求与 fail-fast 说明"
else
  bad "契约 4：compose.prod.yml 的 BFF_TRUST_APISIX 要求/说明缺失"
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ] || exit 1
exit 0

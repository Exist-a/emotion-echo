#!/usr/bin/env bash
# scripts/test_bff_jwt_secret.sh
#
# web-bff 必须设 BFF_JWT_SECRET env (Stage 94 PR-5 §P0-10 回归)
#
# 背景：Stage 94 PR-5 §P0-10 删除 web-bff config.go Auth.JWTSecret 默认值,
# 空字符串触发 log.Fatal "auth: JWT secret must not be empty" fail-fast。
# 但 docker-compose.apps.yml web-bff block 历史上只设 INTERNAL_API_KEY 等,
# 没设 BFF_JWT_SECRET —— Stage 32 PR-14 注释说 "由 APISIX 统一管理" 误导。
# 实测 dev 模式启动 web-bff 反复重启, 日志显示 "JWT secret must not be empty"。
#
# 修复 (2026-09-16): web-bff block 加 BFF_JWT_SECRET: ${BFF_JWT_SECRET:-dev-bff-secret}
# 与 apisix-seed block 一致 (line 699), dev 默认 dev-bff-secret 让 APISIX
# jwt-auth consumer 与 BFF 用同一密钥验签; prod 由 .env.local 注入真随机 secret。
#
# 验证项：
#   1. web-bff block 必须含 BFF_JWT_SECRET env
#   2. 默认值必须非空（避免 fallback 到空字符串触发 fail-fast）
#   3. 与 apisix-seed block 的 BFF_JWT_SECRET 值一致（同密钥验签）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
APPS_FILE="$SCRIPT_DIR/../deploy/docker-compose.apps.yml"

pass=0
fail=0

# 用 python 解析 yaml（避免 shell 转义 + 嵌套引号问题）
read -r -d '' PY_SCRIPT <<'PYEOF'
import sys
import re

APPS = sys.argv[1]

with open(APPS, "r", encoding="utf-8") as f:
    text = f.read()

# 简单 yaml block 提取（service key 下到下一个同级 service key）
def extract(svc):
    lines = text.split("\n")
    out = []
    in_block = False
    for line in lines:
        if re.match(rf"^  {re.escape(svc)}:", line):
            in_block = True
            out.append(line)
            continue
        if in_block:
            if re.match(r"^  [a-zA-Z0-9_-]+:", line):
                break
            out.append(line)
    return "\n".join(out)

def extract_default(block, var):
    """Extract the default value from ${VAR:-default} or quoted literal."""
    m = re.search(rf"{re.escape(var)}:\s*\"?\${{{re.escape(var)}:-([^}}]+)}}\"?", block)
    return m.group(1) if m else None

bff = extract("emotion-echo-web-bff")
seed = extract("emotion-echo-apisix-seed")

# Test 1
print("PRESENT_BFF_JWT_SECRET" if "BFF_JWT_SECRET:" in bff else "MISSING_BFF_JWT_SECRET")

# Test 2
m = re.search(r"BFF_JWT_SECRET:\s*\"?\$\{BFF_JWT_SECRET:-([^}]+)}\"?", bff)
if m:
    val = m.group(1).strip()
    print(f"DEFAULT_BFF={val}")
else:
    print("MISSING_DEFAULT_BFF")

# Test 3
bff_default = m.group(1).strip() if m else None
m2 = re.search(r"BFF_JWT_SECRET:\s*\"?\$\{BFF_JWT_SECRET:-([^}]+)}\"?", seed)
seed_default = m2.group(1).strip() if m2 else None
print(f"DEFAULT_SEED={seed_default}")
print(f"EQUAL={'yes' if bff_default == seed_default else 'no'}")
PYEOF

result=$(python -c "$PY_SCRIPT" "$APPS_FILE")
PRESENT=$(echo "$result" | grep "^PRESENT" | head -1)
DEFAULT_BFF=$(echo "$result" | grep "^DEFAULT_BFF=" | head -1 | cut -d= -f2)
DEFAULT_SEED=$(echo "$result" | grep "^DEFAULT_SEED=" | head -1 | cut -d= -f2)
EQUAL=$(echo "$result" | grep "^EQUAL=" | head -1 | cut -d= -f2)

echo "=== TDD: web-bff 必须设 BFF_JWT_SECRET (Stage 94 PR-5 §P0-10 回归) ==="
echo

echo "--- 1) web-bff block 含 BFF_JWT_SECRET env ---"
if [ "$PRESENT" = "PRESENT_BFF_JWT_SECRET" ]; then
  echo "  ✓ web-bff block 含 BFF_JWT_SECRET env"
  pass=$((pass + 1))
else
  echo "  ✗ web-bff block 缺 BFF_JWT_SECRET env (Stage 94 PR-5 §P0-10 fail-fast 触发)"
  fail=$((fail + 1))
fi

echo
echo "--- 2) BFF_JWT_SECRET 默认值非空 ---"
if [ -n "$DEFAULT_BFF" ] && [ "$DEFAULT_BFF" != "" ]; then
  echo "  ✓ BFF_JWT_SECRET 默认值 = $DEFAULT_BFF (非空)"
  pass=$((pass + 1))
else
  echo "  ✗ BFF_JWT_SECRET 默认值为空（会触发 log.Fatal）"
  fail=$((fail + 1))
fi

echo
echo "--- 3) web-bff 与 apisix-seed JWT secret 默认值一致 ---"
if [ "$EQUAL" = "yes" ]; then
  echo "  ✓ web-bff 与 apisix-seed 默认 secret 一致 ($DEFAULT_BFF)"
  pass=$((pass + 1))
else
  echo "  ✗ web-bff 默认 ($DEFAULT_BFF) vs apisix-seed 默认 ($DEFAULT_SEED) 不一致"
  echo "    （dev 模式会 JWT 验签失败）"
  fail=$((fail + 1))
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0

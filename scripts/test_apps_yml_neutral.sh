#!/usr/bin/env bash
# scripts/test_apps_yml_neutral.sh
#
# PR-ENV-2 RED 阶段测试：验证 apps.yml 中性化
#
# 验证项：
#   1. NACOS_NAMESPACE 在所有 svc 都是 ${NACOS_NAMESPACE:-...} 形式
#   2. SKYWALKING_ENABLED 在所有 svc 都是 ${SKYWALKING_ENABLED:-...} 形式
#   3. KAFKA_ENABLED 在 chat-svc 是 ${KAFKA_ENABLED:-...} 形式
#   4. BFF 的 BFF_DEV_RETURN_CODE / BFF_TRUST_APISIX 已是 ${VAR:-default} 形式
#   5. NUXT_PUBLIC_API_BASE_URL 在 web svc 是 ${VAR:-default} 形式
#   6. 中性化后 docker compose config 语法仍合法
#   7. 中性化后 + compose.dev.yml 启动 config 仍合法

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR/../deploy"
APPS_FILE="$DEPLOY_DIR/docker-compose.apps.yml"

pass=0
fail=0

assert_not_hardcoded() {
  local file="$1"
  local pattern="$2"   # 硬编码值（如 SKYWALKING_ENABLED: "true"）
  local desc="$3"
  if grep -qE "$pattern" "$file"; then
    echo "  ✗ $desc (发现硬编码: $pattern)"
    fail=$((fail + 1))
  else
    echo "  ✓ $desc"
    pass=$((pass + 1))
  fi
}

assert_uses_default() {
  local file="$1"
  local var="$2"
  local desc="$3"
  # 使用单引号 heredoc 风格避免 bash 把 ${...:-...} 当变量展开
  local pattern
  pattern='\$\{'${var}':-'
  if grep -qE "$pattern" "$file"; then
    echo "  ✓ $desc (含 \$"$"{"$var":-})"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (缺 \$"$"{"$var":-})"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: apps.yml 中性化验证 ==="
echo

echo "--- 1) NACOS_NAMESPACE 应全部用 \${NACOS_NAMESPACE:-...} ---"
# 当前硬编码模式：NACOS_NAMESPACE: "emotion-echo-dev"
# 已被中性化后应只剩：NACOS_NAMESPACE: "${NACOS_NAMESPACE:-emotion-echo-dev}"
HARD_NACOS='NACOS_NAMESPACE:\s*"emotion-echo-dev"'
if grep -qE "$HARD_NACOS" "$APPS_FILE"; then
  echo "  ✗ 仍有硬编码 NACOS_NAMESPACE 出现（应改为 \${NACOS_NAMESPACE:-emotion-echo-dev}）"
  echo "    命中："
  grep -nE "$HARD_NACOS" "$APPS_FILE" | sed 's/^/      /'
  fail=$((fail + 1))
else
  echo "  ✓ NACOS_NAMESPACE 已全部中性化"
  pass=$((pass + 1))
fi

echo
echo "--- 2) SKYWALKING_ENABLED 应全部用 \${SKYWALKING_ENABLED:-true} ---"
HARD_SK='SKYWALKING_ENABLED:\s*"true"'
if grep -qE "$HARD_SK" "$APPS_FILE"; then
  echo "  ✗ 仍有硬编码 SKYWALKING_ENABLED: \"true\""
  echo "    命中："
  grep -nE "$HARD_SK" "$APPS_FILE" | sed 's/^/      /'
  fail=$((fail + 1))
else
  echo "  ✓ SKYWALKING_ENABLED 已全部中性化"
  pass=$((pass + 1))
fi

echo
echo "--- 3) KAFKA_ENABLED 在 chat-svc 应 \${KAFKA_ENABLED:-...} ---"
HARD_KAFKA='KAFKA_ENABLED:\s*"true"'
if grep -qE "$HARD_KAFKA" "$APPS_FILE"; then
  echo "  ✗ chat-svc 仍有硬编码 KAFKA_ENABLED: \"true\""
  grep -nE "$HARD_KAFKA" "$APPS_FILE" | sed 's/^/      /'
  fail=$((fail + 1))
else
  echo "  ✓ KAFKA_ENABLED 已中性化"
  pass=$((pass + 1))
fi

echo
echo "--- 4) NUXT_PUBLIC_API_BASE_URL 在 web 应 \${NUXT_PUBLIC_API_BASE_URL:-...} ---"
HARD_NUXT='NUXT_PUBLIC_API_BASE_URL:\s*http://localhost:19080/api/v1'
if grep -qE "$HARD_NUXT" "$APPS_FILE"; then
  echo "  ✗ emotion-echo-web 仍有硬编码 NUXT_PUBLIC_API_BASE_URL"
  grep -nE "$HARD_NUXT" "$APPS_FILE" | sed 's/^/      /'
  fail=$((fail + 1))
else
  echo "  ✓ NUXT_PUBLIC_API_BASE_URL 已中性化"
  pass=$((pass + 1))
fi

echo
echo "--- 5) BFF 已用 \${VAR:-default} 形式的项 ---"
assert_uses_default "$APPS_FILE" "BFF_DEV_RETURN_CODE" "BFF_DEV_RETURN_CODE"
assert_uses_default "$APPS_FILE" "BFF_TRUST_APISIX" "BFF_TRUST_APISIX"

echo
echo "--- 6) 中性化后 docker compose config 语法仍合法 ---"
cd "$DEPLOY_DIR"
if docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml config --quiet 2>/dev/null; then
  echo "  ✓ base 链路 (infra+apps) 语法合法"
  pass=$((pass + 1))
else
  echo "  ✗ base 链路 (infra+apps) 语法错误"
  docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml config 2>&1 | tail -5
  fail=$((fail + 1))
fi

if docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml config --quiet 2>/dev/null; then
  echo "  ✓ dev 链路 (infra+apps+dev) 语法合法"
  pass=$((pass + 1))
else
  echo "  ✗ dev 链路 (infra+apps+dev) 语法错误"
  docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml config 2>&1 | tail -5
  fail=$((fail + 1))
fi
cd - > /dev/null

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
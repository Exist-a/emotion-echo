#!/usr/bin/env bash
# scripts/test_compose_override.sh
#
# PR-ENV-1 RED 阶段测试：验证 ADR-20 C 方案结构
#
# 验证项：
#   1. deploy/compose.dev.yml 存在且结构合法（docker compose config 不报错）
#   2. dev.yml 覆盖 BFF_DEV_RETURN_CODE=1 + BFF 8894:8894 端口映射
#   3. dev.yml 覆盖前端 NUXT_PUBLIC_API_BASE_URL=http://localhost:19080
#   4. dev.yml 不影响 infra.yml/apps.yml 启动路径（base 链路仍正常）
#   5. dev.yml 没引用 apps.yml 没定义的服务（避免无效覆盖）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR/../deploy"
DEV_FILE="$DEPLOY_DIR/compose.dev.yml"
APPS_FILE="$DEPLOY_DIR/docker-compose.apps.yml"
INFRA_FILE="$DEPLOY_DIR/docker-compose.infra.yml"

pass=0
fail=0

assert_file_exists() {
  local file="$1"
  local desc="$2"
  if [ -f "$file" ]; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (missing: $file)"
    fail=$((fail + 1))
  fi
}

assert_contains() {
  local file="$1"
  local pattern="$2"
  local desc="$3"
  if grep -qE "$pattern" "$file"; then
    echo "  ✓ $desc"
    pass=$((pass + 1))
  else
    echo "  ✗ $desc (pattern: $pattern in $file)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: ADR-20 compose.dev.yml 覆盖层验证 ==="
echo

echo "--- 1) 文件存在性 ---"
assert_file_exists "$DEV_FILE" "compose.dev.yml 已建"
assert_file_exists "$APPS_FILE" "docker-compose.apps.yml 存在（基线）"
assert_file_exists "$INFRA_FILE" "docker-compose.infra.yml 存在（基线）"

echo
echo "--- 2) dev.yml 覆盖关键 dev 假设 ---"
# BFF 8894 端口映射
assert_contains "$DEV_FILE" "8894:8894" \
  "dev.yml 覆盖 BFF 8894:8894 端口映射"

# BFF_DEV_RETURN_CODE=1（dev 调试便利）
assert_contains "$DEV_FILE" "BFF_DEV_RETURN_CODE" \
  "dev.yml 含 BFF_DEV_RETURN_CODE 环境变量"

# BFF_TRUST_APISIX=true（dev 直连调试）
assert_contains "$DEV_FILE" "BFF_TRUST_APISIX" \
  "dev.yml 含 BFF_TRUST_APISIX 配置"

# 前端 NUXT_PUBLIC_API_BASE_URL 指向 localhost:19080
assert_contains "$DEV_FILE" "NUXT_PUBLIC_API_BASE_URL" \
  "dev.yml 含 NUXT_PUBLIC_API_BASE_URL 配置"

# CORS 允许 localhost:3000（Nuxt dev server）
assert_contains "$DEV_FILE" "CORS_ALLOW_ORIGINS" \
  "dev.yml 含 CORS_ALLOW_ORIGINS 配置"

echo
echo "--- 3) docker compose config 语法合法性 ---"
if [ -f "$DEV_FILE" ]; then
  cd "$DEPLOY_DIR"
  if docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml config --quiet 2>/dev/null; then
    echo "  ✓ docker compose config (infra+apps+dev) 语法合法"
    pass=$((pass + 1))
  else
    echo "  ✗ docker compose config (infra+apps+dev) 语法错误："
    docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml config 2>&1 | tail -5
    fail=$((fail + 1))
  fi
  cd - > /dev/null
else
  echo "  ⊘ docker compose config 跳过（dev.yml 不存在）"
fi

echo
echo "--- 4) dev.yml 不引入新服务（只覆盖既有 services） ---"
if [ -f "$DEV_FILE" ]; then
  # 提取 dev.yml 里 services: 下的服务名 + apps.yml 里的服务名，比对
  DEV_SVCS=$(grep -E "^  [a-z-]+:$" "$DEV_FILE" | sed 's/^  //;s/://' | sort)
  APPS_SVCS=$(grep -E "^  [a-z-]+:$" "$APPS_FILE" | sed 's/^  //;s/://' | sort)
  echo "  dev.yml 中声明的服务："
  echo "$DEV_SVCS" | sed 's/^/    /'
  echo "  apps.yml 中存在的服务："
  echo "$APPS_SVCS" | sed 's/^/    /'

  EXTRA=$(comm -23 <(echo "$DEV_SVCS") <(echo "$APPS_SVCS"))
  if [ -z "$EXTRA" ]; then
    echo "  ✓ dev.yml 没有引入 apps.yml 之外的服务"
    pass=$((pass + 1))
  else
    echo "  ✗ dev.yml 引入了 apps.yml 没定义的服务："
    echo "$EXTRA" | sed 's/^/    /'
    fail=$((fail + 1))
  fi
else
  echo "  ⊘ 跳过（dev.yml 不存在）"
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
#!/usr/bin/env bash
# scripts/test_docs_update.sh
#
# PR-ENV-4 RED 阶段测试：验证 configuration.md 与 QUICKSTART.md 更新
#
# 验证项：
#   1. deploy/configuration.md 存在
#   2. 列出 dev 启动命令（带 -f compose.dev.yml）
#   3. 列出 prod 启动命令（带 -f compose.prod.yml）
#   4. env 变量清单覆盖 5 大类（命名空间/业务开关/BFF/Web/CORS）
#   5. dev vs prod 关键差异表存在
#   6. 关联 ADR-20 文件名
#   7. QUICKSTART.md 已加 -f compose.dev.yml 启动命令
#   8. QUICKSTART.md 引用 deploy/configuration.md

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$SCRIPT_DIR/.."
CONFIG_FILE="$ROOT_DIR/deploy/configuration.md"
QUICKSTART="$ROOT_DIR/QUICKSTART.md"

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
    echo "  ✗ $desc (pattern: $pattern)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: configuration.md + QUICKSTART.md 更新验证 ==="
echo

echo "--- 1) 文件存在性 ---"
assert_file_exists "$CONFIG_FILE" "deploy/configuration.md 已建"
assert_file_exists "$QUICKSTART" "QUICKSTART.md 存在"

echo
echo "--- 2) configuration.md 关键章节 ---"
assert_contains "$CONFIG_FILE" "compose\.dev\.yml" "configuration.md 提及 compose.dev.yml"
assert_contains "$CONFIG_FILE" "compose\.prod\.yml" "configuration.md 提及 compose.prod.yml"
assert_contains "$CONFIG_FILE" "NACOS_NAMESPACE" "configuration.md 含 NACOS_NAMESPACE"
assert_contains "$CONFIG_FILE" "KAFKA_ENABLED" "configuration.md 含 KAFKA_ENABLED"
assert_contains "$CONFIG_FILE" "BFF_DEV_RETURN_CODE" "configuration.md 含 BFF_DEV_RETURN_CODE"
assert_contains "$CONFIG_FILE" "NUXT_PUBLIC_API_BASE_URL" "configuration.md 含 NUXT_PUBLIC_API_BASE_URL"
assert_contains "$CONFIG_FILE" "CORS_ALLOW_ORIGINS" "configuration.md 含 CORS_ALLOW_ORIGINS"
assert_contains "$CONFIG_FILE" "adr-2026-09-env-profile-strategy" "configuration.md 引用 ADR-20"

echo
echo "--- 3) QUICKSTART.md 已加 dev override 启动命令 ---"
assert_contains "$QUICKSTART" "compose\.dev\.yml" "QUICKSTART.md 含 compose.dev.yml"
assert_contains "$QUICKSTART" "deploy/configuration\.md" "QUICKSTART.md 引用 configuration.md"
assert_contains "$QUICKSTART" "ADR-20" "QUICKSTART.md 提及 ADR-20"

echo
echo "--- 4) QUICKSTART.md 已移除旧的硬启动命令（不带 -f compose.dev.yml 的旧命令） ---"
# 旧的："docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml up -d"
# 新的："docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml up -d"
# 测试：旧命令（没有 dev.yml）在 §步骤 4 应已替换
# 取 §步骤 4 后续内容检查
OLD_HARDCODE='docker compose -f docker-compose\.infra\.yml -f docker-compose\.apps\.yml up -d --no-build'
if grep -qE "$OLD_HARDCODE" "$QUICKSTART"; then
  echo "  ✗ QUICKSTART.md 仍有旧硬启动命令（未加 -f compose.dev.yml）"
  grep -nE "$OLD_HARDCODE" "$QUICKSTART" | sed 's/^/      /'
  fail=$((fail + 1))
else
  echo "  ✓ QUICKSTART.md 已用新启动命令（含 -f compose.dev.yml）"
  pass=$((pass + 1))
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
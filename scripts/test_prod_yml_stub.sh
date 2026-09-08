#!/usr/bin/env bash
# scripts/test_prod_yml_stub.sh
#
# PR-ENV-3 RED 阶段测试：验证 compose.prod.yml 空壳占位
#
# 决策依据（ADR-20 §六）：
#   选空壳而非完整 prod 期望，避免写未验证的假设（决策 18 §四 警告）。
#   远端真部署时再补具体值。
#
# 验证项：
#   1. deploy/compose.prod.yml 存在
#   2. 含 ADR 引用注释（指向 adr-2026-09-env-profile-strategy.md）
#   3. 含 TODO 注释（提示远端部署时填充差异项）
#   4. docker compose config (infra+apps+prod) 语法合法
#   5. 不引入新服务（只覆盖既有 services）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEPLOY_DIR="$SCRIPT_DIR/../deploy"
PROD_FILE="$DEPLOY_DIR/compose.prod.yml"
APPS_FILE="$DEPLOY_DIR/docker-compose.apps.yml"

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

echo "=== TDD: compose.prod.yml 空壳占位验证 ==="
echo

echo "--- 1) 文件存在性 ---"
assert_file_exists "$PROD_FILE" "compose.prod.yml 已建"

echo
echo "--- 2) 含 ADR 引用 ---"
assert_contains "$PROD_FILE" "adr-2026-09-env-profile-strategy" "引用 ADR-20 文件名"
assert_contains "$PROD_FILE" "TODO" "含 TODO 提示"

echo
echo "--- 3) 含 ADR-20 §三差异清单关键项 ---"
# 至少提及部分关键差异项作为占位提示
PROD_KEYS=("NACOS_NAMESPACE" "KAFKA_ENABLED" "JWT" "TLS" "resource" "limit")
for key in "${PROD_KEYS[@]}"; do
  if grep -qiE "$key" "$PROD_FILE" 2>/dev/null; then
    echo "  ✓ 含 '$key' 提示"
    pass=$((pass + 1))
  else
    echo "  ⊘ 未含 '$key'（占位不强求全部）"
  fi
done

echo
echo "--- 4) docker compose config 语法合法 ---"
if [ -f "$PROD_FILE" ]; then
  cd "$DEPLOY_DIR"
  if docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.prod.yml config --quiet 2>/dev/null; then
    echo "  ✓ prod 链路 (infra+apps+prod) 语法合法"
    pass=$((pass + 1))
  else
    echo "  ✗ prod 链路语法错误："
    docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.prod.yml config 2>&1 | tail -5
    fail=$((fail + 1))
  fi
  cd - > /dev/null
fi

echo
echo "--- 5) 不引入新服务 ---"
if [ -f "$PROD_FILE" ]; then
  # 包含下划线开头的服务名（如 _prod_overrides_pending）
  PROD_SVCS=$(grep -E "^  [a-zA-Z_-]+:$" "$PROD_FILE" 2>/dev/null | sed 's/^  //;s/://' | sort)
  APPS_SVCS=$(grep -E "^  [a-zA-Z_-]+:$" "$APPS_FILE" | sed 's/^  //;s/://' | sort)
  # 过滤掉下划线开头的占位服务（约定俗成）
  PROD_REAL=$(echo "$PROD_SVCS" | grep -v "^_" || true)
  if [ -z "$PROD_REAL" ]; then
    echo "  ✓ prod.yml 真实服务声明为空（仅占位 _xxx_ 服务，可接受）"
    pass=$((pass + 1))
  else
    EXTRA=$(comm -23 <(echo "$PROD_REAL") <(echo "$APPS_SVCS"))
    if [ -z "$EXTRA" ]; then
      echo "  ✓ 没有引入 apps.yml 之外的真实服务"
      pass=$((pass + 1))
    else
      echo "  ✗ 引入了 apps.yml 没定义的服务：$EXTRA"
      fail=$((fail + 1))
    fi
  fi
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
#!/usr/bin/env bash
# scripts/test_check_minio_health.sh — check_minio_health.sh 结构守卫（E2E-27 #2 / F-h）
#
# 背景：check_minio_health.sh（4 契约）与 smoke_upload_minio.sh 此前均**未接 CI**
# （AP-10 同型：守卫写了不接线 = 没写）。E2E-27 #2 把两守卫接入 e2e-guards——
# smoke 的结构守卫已存在（test_smoke_upload_minio.sh），本脚本补 MinIO 健康守卫的
# 结构守卫（纯静态，CI 无容器栈也能跑；运行时 4 契约在本地/dev 模式验）。
#
# 验证项：
#   1. scripts/check_minio_health.sh 存在且可执行
#   2. 含 4 项契约断言：容器 running / liveness 200 / console 可达 / avatars 桶存在
#   3. 退出码定义清晰（0 全过 / 1 有 FAIL）
#   4. 前置：容器未运行时报错退出（不默默通过）
#   5. bash 语法合法
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
# 用法：bash scripts/test_check_minio_health.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/check_minio_health.sh"

pass=0
fail=0

assert_file() {
  if [ -f "$1" ]; then
    echo "  ✓ $2"
    pass=$((pass + 1))
  else
    echo "  ✗ $2 (missing)"
    fail=$((fail + 1))
  fi
}

assert_executable() {
  if [ -x "$1" ]; then
    echo "  ✓ $2"
    pass=$((pass + 1))
  else
    echo "  ✗ $2 (not executable)"
    fail=$((fail + 1))
  fi
}

assert_contains() {
  if grep -qE "$2" "$1"; then
    echo "  ✓ $3"
    pass=$((pass + 1))
  else
    echo "  ✗ $3 (pattern: $2)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: check_minio_health.sh 结构验证 ==="
echo

echo "--- 1) 文件存在 + 可执行 ---"
assert_file "$SCRIPT" "scripts/check_minio_health.sh 存在"
assert_executable "$SCRIPT" "check_minio_health.sh 可执行"

echo
echo "--- 2) 4 项契约断言在位 ---"
assert_contains "$SCRIPT" 'contract 1' "契约 1: 容器 running"
assert_contains "$SCRIPT" 'minio/health/live' "契约 2: liveness 探针"
assert_contains "$SCRIPT" '9001' "契约 3: console 可达"
assert_contains "$SCRIPT" 'avatars' "契约 4: avatars 桶存在"

echo
echo "--- 3) 退出码 ---"
assert_contains "$SCRIPT" 'exit 0' "成功路径 exit 0"
assert_contains "$SCRIPT" 'exit 1' "失败路径 exit 1"

echo
echo "--- 4) 前置容器检查（未运行必须报错而非默默通过） ---"
assert_contains "$SCRIPT" 'docker inspect' "前置: docker inspect 状态检查"
assert_contains "$SCRIPT" '未运行' "前置: 未运行报错文案"

echo
echo "--- 5) bash 语法合法 ---"
if bash -n "$SCRIPT" 2>/dev/null; then
  echo "  ✓ bash -n 通过"
  pass=$((pass + 1))
else
  echo "  ✗ bash -n 报错"
  fail=$((fail + 1))
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0

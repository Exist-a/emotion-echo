#!/usr/bin/env bash
# scripts/test_smoke_upload_minio.sh
#
# PR-UP-3 §契约 8 smoke 脚本结构验证
#
# 验证项：
#   1. scripts/smoke_upload_minio.sh 存在且可执行
#   2. 含 4 项契约断言：HTTP 200 / url 非空 / url HEAD 可达 / mc ls 看到文件
#   3. 退出码定义清晰（0/1）
#   4. 含 APISIX_URL / USER_ID 环境变量支持
#   5. 触发前置容器检查（容器未运行时报错而非默默通过）
#   6. E2E-27 #2 三缺陷防回退：cwd 临时文件（native curl 读不到 /tmp）/
#      登录取 Bearer（uploads 路由 jwt-auth）/ mc 走 MC_HOST_（免 alias 空列）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/smoke_upload_minio.sh"

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

echo "=== TDD: smoke_upload_minio.sh 契约结构验证 ==="
echo

echo "--- 1) 文件存在 + 可执行 ---"
assert_file "$SCRIPT" "scripts/smoke_upload_minio.sh 存在"
assert_executable "$SCRIPT" "smoke_upload_minio.sh 可执行"

echo
echo "--- 2) 含 4 项契约断言 ---"
assert_contains "$SCRIPT" "/api/v1/uploads/image" "契约 1: POST /api/v1/uploads/image"
assert_contains "$SCRIPT" '"url"' "契约 1: 响应含 url 字段"
assert_contains "$SCRIPT" 'curl [^|]*-I' "契约 2: HEAD url 可达"
assert_contains "$SCRIPT" 'mc ls' "契约 3: mc ls avatars/uploads/"

echo
echo "--- 3) 退出码 ---"
# exit 在脚本中可能是行首或缩进 2/4 格
assert_contains "$SCRIPT" '(^|[[:space:]])exit 0' "成功路径 exit 0"
assert_contains "$SCRIPT" '(^|[[:space:]])exit 1' "失败路径 exit 1"

echo
echo "--- 4) 环境变量支持 ---"
assert_contains "$SCRIPT" 'APISIX_URL' "支持 APISIX_URL 覆盖"
assert_contains "$SCRIPT" 'USER_ID' "支持 USER_ID 覆盖"

echo
echo "--- 5) 前置容器检查 ---"
assert_contains "$SCRIPT" 'emotion-echo-apisix' "前置: APISIX 容器"
assert_contains "$SCRIPT" 'emotion-echo-web-bff' "前置: BFF 容器"
assert_contains "$SCRIPT" 'emotion-echo-minio' "前置: MinIO 容器"

echo
echo "--- 6) E2E-27 #2 三缺陷防回退（2026-10-03 运行时实测）---"
# 缺陷 1：TMP_IMG=/tmp/... 时 mingw curl rc26 读不到（HTTP 000000）——必须 cwd 相对路径
assert_contains "$SCRIPT" 'TMP_IMG="\./' "缺陷1: 临时文件为 cwd 相对路径（native curl 读不到 /tmp）"
assert_contains "$SCRIPT" 'trap .*rm -f' "缺陷1b: trap 清理临时文件（cwd 不留残留）"
# 缺陷 2：POST /uploads/image 经 APISIX jwt-auth，只带 X-User-Id 必 401
assert_contains "$SCRIPT" 'auth/login' "缺陷2: 开头登录取 token"
assert_contains "$SCRIPT" 'Authorization: Bearer' "缺陷2b: 上传请求带 Bearer"
# 缺陷 3：docker run 裸 mc ls 无 alias ⇒ 空输出 rc=0 ⇒ 契约 3 永远 FAIL（从未真验过）
assert_contains "$SCRIPT" 'MC_HOST_' "缺陷3: mc 走 MC_HOST_ 环境变量（免 alias 静默空列）"

echo
echo "--- 7) bash 语法合法 ---"
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

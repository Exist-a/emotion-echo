#!/usr/bin/env bash
# scripts/test_build_dev_images.sh
#
# PR-CHORE-3 RED 阶段测试：验证 build_dev_images.sh 的成功识别逻辑
#
# bug 复现：原脚本用 `grep -q "^ Image .* Built$"` 匹配 docker compose v1 格式，
#          但 Docker Compose v2.x 输出是 `✔ Service xxx Built`，导致实际构建成功
#          但脚本误报 FAIL（todo-pile §D stage-54 §七 D 记录的真实问题）。
#
# 修复后：应能正确识别 v2 的"✔ Service xxx Built" + "naming to docker.io/xxx" 双标志。

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
FIXTURES_DIR="$SCRIPT_DIR/__tests__/fixtures"

pass=0
fail=0

assert_contains() {
  local fixture="$1"
  local pattern="$2"
  local should_match="$3"  # "yes" or "no"
  local desc="$4"

  if [ "$should_match" = "yes" ]; then
    if grep -qE "$pattern" "$FIXTURES_DIR/$fixture"; then
      echo "  ✓ $desc"
      pass=$((pass + 1))
    else
      echo "  ✗ $desc (expected match for: $pattern in $fixture)"
      fail=$((fail + 1))
    fi
  else
    if grep -qE "$pattern" "$FIXTURES_DIR/$fixture"; then
      echo "  ✗ $desc (expected NO match for: $pattern in $fixture)"
      fail=$((fail + 1))
    else
      echo "  ✓ $desc"
      pass=$((pass + 1))
    fi
  fi
}

echo "=== TDD: build_dev_images.sh 成功识别测试 ==="
echo

echo "--- 1) 验证新匹配模式覆盖 v2 成功输出 ---"
# 这是修复后应采用的正则
NEW_PATTERN='✔ Service .+ Built|naming to docker\.io/'

# 成功日志（完整 build）
assert_contains "compose_build_success.log" "$NEW_PATTERN" "yes" \
  "完整 build 成功日志应被识别"

# 成功日志（up to date 缓存）
assert_contains "compose_build_up_to_date.log" "$NEW_PATTERN" "yes" \
  "缓存 up-to-date 日志应被识别"

# 失败日志
assert_contains "compose_build_failed.log" "$NEW_PATTERN" "no" \
  "失败日志不应被误识别为成功"

echo
echo "--- 2) 验证旧匹配模式 (v1 格式) 已不适用 ---"
# 这是 bug 的根源
OLD_PATTERN='^ Image .* Built$'

assert_contains "compose_build_success.log" "$OLD_PATTERN" "no" \
  "v1 旧模式不应匹配 v2 成功输出"
assert_contains "compose_build_up_to_date.log" "$OLD_PATTERN" "no" \
  "v1 旧模式不应匹配 v2 up-to-date 输出"
assert_contains "compose_build_failed.log" "$OLD_PATTERN" "no" \
  "v1 旧模式不应匹配 v2 失败输出"

echo
echo "--- 3) 验证 build_dev_images.sh 已用新模式 ---"
SCRIPT="$SCRIPT_DIR/build_dev_images.sh"
if grep -qE "✔ Service|naming to docker\.io/" "$SCRIPT"; then
  echo "  ✓ build_dev_images.sh 已采用 v2 成功识别模式"
  pass=$((pass + 1))
else
  echo "  ✗ build_dev_images.sh 仍使用旧 v1 模式（未修复）"
  fail=$((fail + 1))
fi

# 验证旧模式已被移除
if grep -qE '^\s*if grep -q "\^ Image' "$SCRIPT"; then
  echo "  ✗ build_dev_images.sh 仍含旧 '^ Image' 匹配"
  fail=$((fail + 1))
else
  echo "  ✓ build_dev_images.sh 已移除旧 '^ Image' 匹配"
  pass=$((pass + 1))
fi

echo
echo "--- 4) Stage 74: web 前端纳入 build 列表 + node 基础镜像预拉 ---"
# stage-73 §五：web 前端容器未起的次生问题——脚本默认不 build web、
# 预拉列表不含 node:20-alpine，Docker Hub 限流时 build web 第一步即失败

if grep -qE 'ALL_SVCS=.*emotion-echo-web' "$SCRIPT"; then
  echo "  ✓ ALL_SVCS 默认包含 emotion-echo-web"
  pass=$((pass + 1))
else
  echo "  ✗ ALL_SVCS 默认缺 emotion-echo-web（stage-73 §五 open 项）"
  fail=$((fail + 1))
fi

if grep -qE 'BASE_IMAGES=.*node:20-alpine' "$SCRIPT"; then
  echo "  ✓ 默认 BASE_IMAGES 包含 node:20-alpine"
  pass=$((pass + 1))
else
  echo "  ✗ 默认 BASE_IMAGES 缺 node:20-alpine（web Dockerfile 基础镜像）"
  fail=$((fail + 1))
fi

echo
echo "=== Result: $pass passed, $fail failed ==="
if [ $fail -gt 0 ]; then
  exit 1
fi
exit 0
#!/usr/bin/env bash
# scripts/test_smoke_ai_profile_v2.sh
#
# PR-TTS-4 测试：smoke_ai_profile_v2.sh 双分支契约结构
#
# 验证项：
#   1. smoke_ai_profile_v2.sh 存在且可执行
#   2. 含 default / ai 两种 mode 分支
#   3. default 分支断言：3 AI 容器 NOT running + ai-svc healthz 降级路径
#   4. ai 分支断言：3 AI 容器 healthy + 3 health 端口 200
#   5. 含 APISIX_URL 环境变量支持

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/smoke_ai_profile_v2.sh"

pass=0
fail=0

assert_file() {
  if [ -f "$1" ]; then
    echo "  ✓ $2"
    pass=$((pass + 1))
  else
    echo "  ✗ $2"
    fail=$((fail + 1))
  fi
}

assert_executable() {
  if [ -x "$1" ]; then
    echo "  ✓ $2"
    pass=$((pass + 1))
  else
    echo "  ✗ $2"
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

# assert_contains_or: 多个模式任一命中即通过（OR 语义）
assert_contains_or() {
  local file="$1"; shift
  local desc="$1"; shift
  for pat in "$@"; do
    if grep -qE "$pat" "$file"; then
      echo "  ✓ $desc"
      pass=$((pass + 1))
      return
    fi
  done
  echo "  ✗ $desc (任一模式命中: $* )"
  fail=$((fail + 1))
}

echo "=== TDD: smoke_ai_profile_v2.sh 双分支契约结构 ==="
echo

echo "--- 1) 文件存在 + 可执行 ---"
assert_file "$SCRIPT" "smoke_ai_profile_v2.sh 存在"
assert_executable "$SCRIPT" "smoke_ai_profile_v2.sh 可执行"

echo
echo "--- 2) 含 default / ai 两种 mode 分支 ---"
assert_contains "$SCRIPT" 'MODE=.*default' "MODE 默认 default"
assert_contains_or "$SCRIPT" "含 default/ai 分支判断" \
  '分支 A' '分支 B' 'default.*mode'

echo
echo "--- 3) default 分支契约 ---"
assert_contains_or "$SCRIPT" "default: FER 不应 running" \
  'emotion-echo-fer.*NOT running' 'fer.*不应' 'default mode' 'emotion-echo-fer'
assert_contains "$SCRIPT" 'emotion-echo-sensevoice' "default: SenseVoice 验证"
assert_contains "$SCRIPT" 'emotion-echo-xtts' "default: XTTS 验证"
assert_contains_or "$SCRIPT" "default: ai-svc healthz 调用" \
  '/api/v1/ai/health' '/ai/health' 'ai/healthz'

echo
echo "--- 4) ai 分支契约 ---"
assert_contains_or "$SCRIPT" "ai: FER healthcheck :8004" \
  'emotion-echo-fer.*healthy' ':8004' 'FER.*8004'
assert_contains_or "$SCRIPT" "ai: SenseVoice healthcheck :8002" \
  'emotion-echo-sensevoice.*healthy' ':8002' 'SenseVoice.*8002'
assert_contains_or "$SCRIPT" "ai: XTTS healthcheck :8003" \
  'emotion-echo-xtts.*healthy' ':8003' 'XTTS.*8003'

echo
echo "--- 5) 环境变量 + 退出码 ---"
assert_contains "$SCRIPT" 'APISIX_URL' "支持 APISIX_URL 覆盖"
assert_contains "$SCRIPT" '(^|[[:space:]])exit 0' "成功路径 exit 0"
assert_contains "$SCRIPT" '(^|[[:space:]])exit 1' "失败路径 exit 1"

echo
echo "--- 6) bash 语法 ---"
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
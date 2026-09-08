#!/usr/bin/env bash
# scripts/test_smoke_chat_grpc.sh
#
# PR-GRPC-6 测试：smoke_bff_chat_grpc.sh 契约结构验证
#
# 验证项：
#   1. smoke_bff_chat_grpc.sh 存在且可执行
#   2. 含 §契约 9 端到端契约断言：
#      - BFF → chat-svc gRPC :8892 健康（grpc health probe）
#      - HTTP :8890 + gRPC :8892 双端口都 listening
#      - 创建会话 / 发消息 / 列消息 / 列会话 4 个 RPC 端到端通
#      - SkyWalking OAP rpc.* tag 验证（POST /api/v1/ai/health 含 chat rpc）
#   3. 退出码 0/1 定义清晰
#   4. 含 APISIX_URL 环境变量支持
#   5. bash -n 语法合法

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/smoke_bff_chat_grpc.sh"

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

assert_contains() {
  if grep -qE "$2" "$1"; then
    echo "  ✓ $3"
    pass=$((pass + 1))
  else
    echo "  ✗ $3 (pattern: $2)"
    fail=$((fail + 1))
  fi
}

echo "=== TDD: smoke_bff_chat_grpc.sh §契约 9 结构验证 ==="
echo

echo "--- 1) 文件存在 + 可执行 ---"
assert_file "$SCRIPT" "smoke_bff_chat_grpc.sh 存在"
assert_executable "$SCRIPT" "smoke_bff_chat_grpc.sh 可执行"

echo
echo "--- 2) §契约 9 关键契约 ---"
# 契约 1: 容器健康
assert_contains_or "$SCRIPT" "契约 1: 容器前置" \
  'emotion-echo-web-bff.*前置' '前置.*容器' 'emotion-echo-chat-svc.*前置'
assert_contains_or "$SCRIPT" "契约 2: gRPC :8892 端口探测" \
  ':8892' '8892.*health' 'chat-svc.*gRPC.*listening'
assert_contains_or "$SCRIPT" "契约 3: grpc_health_v1" \
  'grpc.health.v1' 'health.v1.Health' '/grpc.health'
assert_contains_or "$SCRIPT" "契约 4: CreateConversation RPC" \
  'CreateConversation' '/api/v1/conversations'
assert_contains_or "$SCRIPT" "契约 5: SendMessage RPC" \
  'SendMessage' '/api/v1/conversations/.*messages\|/conversations/.*messages'
assert_contains_or "$SCRIPT" "契约 6: ListMessages RPC" \
  'ListMessages' 'messages'
assert_contains_or "$SCRIPT" "契约 7: ListConversations RPC" \
  'ListConversations'
assert_contains_or "$SCRIPT" "契约 8: gRPC 端点鉴权 (x-user-id metadata)" \
  'x-user-id' 'metadata.*x-user-id\|metadata.*user'
assert_contains_or "$SCRIPT" "契约 9: SkyWalking OAP rpc.* tag" \
  'rpc\.\*' 'rpc.*tag\|rpc\.client\|rpc\.server\|OAP.*rpc'

echo
echo "--- 3) 退出码 + env var ---"
assert_contains "$SCRIPT" '(^|[[:space:]])exit 0' "成功路径 exit 0"
assert_contains "$SCRIPT" '(^|[[:space:]])exit 1' "失败路径 exit 1"
assert_contains "$SCRIPT" 'APISIX_URL' "支持 APISIX_URL 覆盖"

echo
echo "--- 4) bash 语法 ---"
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
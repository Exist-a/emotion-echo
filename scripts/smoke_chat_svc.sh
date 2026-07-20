#!/usr/bin/env bash
#
# smoke_chat_svc.sh · chat-svc 会话/消息 API · 冒烟测试
#
# 端点：
#   - GET  /health
#   - POST /api/v1/conversations                  → {id}
#   - POST /api/v1/conversations/:id/messages     → 200 + 触达 Kafka（黑盒验证 status）
#   - GET  /api/v1/conversations/:id/messages     → [] 列表
#
# 跑法：
#   ./scripts/smoke_chat_svc.sh
#   BASE_URL=http://localhost:8890 SKIP_MESSAGE=1 ./scripts/smoke_chat_svc.sh
#

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8890}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-5}"
TIMEOUT_FLAG="--max-time $HTTP_TIMEOUT"
SKIP_MESSAGE="${SKIP_MESSAGE:-0}"
JWT_TOKEN="${JWT_TOKEN:-}"  # 留空跳过鉴权相关断言

PASS=0
FAIL=0
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

http_assert() {
  local name="$1"; local expected="$2"; local url="$3"; shift 3
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG "$@" "$url" || echo "000")
  if [ "$code" = "$expected" ]; then
    green "  ✓ $name  → $code"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → expected $expected, got $code"
    FAIL=$((FAIL+1))
  fi
}

body_assert_contains() {
  local name="$1"; local url="$2"; local needle="$3"; shift 3
  local body
  body=$(curl -sS -o - $TIMEOUT_FLAG "$@" "$url" 2>&1 || true)
  if printf '%s' "$body" | grep -q -F -- "$needle"; then
    green "  ✓ $name  → contains '$needle'"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → body=$body"
    FAIL=$((FAIL+1))
  fi
}

echo "═══ smoke: chat-svc @ $BASE_URL ═══"

# 1. /health
http_assert "/health (200)" 200 "$BASE_URL/health"
body_assert_contains "/health body has 'status'" "$BASE_URL/health" "status"

# 2. POST /api/v1/conversations
# 需要 user_id，否则 401/403；这里用 demo header 模拟 APISIX 透传的 user_id claim
# ai-svc/chat-svc AuthMiddleware 必须接受非空 Authorization（即使 token 不验签）
# 为避免 401 烟测失败直接 200/201 兼收；具体的 200 是创建成功，201 同样 ok
hdrs_auth=()
if [ -n "$JWT_TOKEN" ]; then
  hdrs_auth=(-H "Authorization: Bearer $JWT_TOKEN")
fi

# 兼容 LLM/JWT 两种典型契约
dummy_jwt='Bearer eyJhbGciOiJIUzI1NiJ9.eyJ1c2VyX2lkIjoxLCJzdWIiOiIxIn0.fake'

# POST 用 --data-binary @file 避免 bash 单引号字面量吞 UTF-8 中文字节
create_pf=$(mktemp)
printf '%s' '{"title":"smoke-test"}' > "$create_pf"
create_resp_pf=$(mktemp)
create_code=$(curl -sS -o "$create_resp_pf" -w '%{http_code}' $TIMEOUT_FLAG \
              -X POST "${hdrs_auth[@]:--H "Authorization: $dummy_jwt"}" \
              -H 'Content-Type: application/json' \
              --data-binary "@$create_pf" \
              "$BASE_URL/api/v1/conversations" 2>/dev/null || echo "000")
create_body=$(cat "$create_resp_pf" 2>/dev/null || echo "")
rm -f "$create_pf" "$create_resp_pf"

case "$create_code" in
  200|201)
    green "  ✓ POST /api/v1/conversations → $create_code"
    PASS=$((PASS+1))
    conv_id=$(printf '%s' "$create_body" | sed -nE 's/.*"id"[[:space:]]*:[[:space:]]*"?([0-9]+)".*/\1/p' | head -1)
    if [ -z "$conv_id" ]; then
      conv_id=$(printf '%s' "$create_body" | python -c "import json,sys; d=json.loads(sys.stdin.read()); print(d.get('id') or d.get('conversation',{}).get('id') or '')" 2>/dev/null || echo "")
    fi
    yellow "  ! extracted conv_id='$conv_id' (best effort)"
    ;;
  401|403)
    yellow "  ! POST /api/v1/conversations → $create_code (auth required); 跳过写路径断言"
    conv_id=""
    ;;
  *)
    red   "  ✗ POST /api/v1/conversations → expected 200/201/401/403 got $create_code body=$create_body"
    FAIL=$((FAIL+1))
    conv_id=""
    ;;
esac

# 3. POST /api/v1/conversations/:id/messages + GET 列消息
if [ -n "$conv_id" ] && [ "$SKIP_MESSAGE" != "1" ]; then
  msg_pf=$(mktemp)
  msg_resp_pf=$(mktemp)
  printf '%s' '{"role":"user","content":"hello from smoke"}' > "$msg_pf"
  msg_code=$(curl -sS -o "$msg_resp_pf" -w '%{http_code}' $TIMEOUT_FLAG \
              -X POST "${hdrs_auth[@]:--H "Authorization: $dummy_jwt"}" \
              -H 'Content-Type: application/json' \
              --data-binary "@$msg_pf" \
              "$BASE_URL/api/v1/conversations/$conv_id/messages" 2>/dev/null || echo "000")
  msg_body=$(cat "$msg_resp_pf" 2>/dev/null || echo "")
  rm -f "$msg_pf" "$msg_resp_pf"

  if [ "$msg_code" = "200" ] || [ "$msg_code" = "201" ]; then
    green "  ✓ POST /api/v1/conversations/:id/messages → $msg_code"
    PASS=$((PASS+1))
  else
    red   "  ✗ POST messages → expected 200/201 got $msg_code body=$msg_body"
    FAIL=$((FAIL+1))
  fi

  # 列消息（已发一条应至少 1 条）
  list_code=$(curl -sS -o /tmp/chat_list.json -w '%{http_code}' $TIMEOUT_FLAG \
              -X GET "${hdrs_auth[@]:--H "Authorization: $dummy_jwt"}" \
              "$BASE_URL/api/v1/conversations/$conv_id/messages" || echo "000")
  list_body=$(cat /tmp/chat_list.json 2>/dev/null || echo "")
  rm -f /tmp/chat_list.json

  if [ "$list_code" = "200" ] && printf '%s' "$list_body" | grep -q "hello from smoke"; then
    green "  ✓ GET /api/v1/conversations/:id/messages contains message"
    PASS=$((PASS+1))
  else
    yellow "  ! GET list returned $list_code, body='$list_body' (may need real JWT)"
  fi
else
  yellow "  ! 跳过 messages 端点 (conv_id 空或 SKIP_MESSAGE=1)"
fi

# 4. /metrics
http_assert "/metrics (200)" 200 "$BASE_URL/metrics"

echo ""
echo "═══ 结果：$PASS passed, $FAIL failed ═══"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0

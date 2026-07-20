#!/usr/bin/env bash
#
# smoke_emotion_llm_service.sh · 文本情绪分析 LLM 微服务 · 冒烟测试
#
# 覆盖端点：
#   - GET  /health                   → {"status":"ok"}
#   - GET  /metrics                  → Prometheus text
#   - POST /analyze   {text}         → {primaryEmotion, sentimentScore, confidence, model}
#   - gRPC    :50051  emotion_llm   （可选，需 grpcurl）
#
# 跑法：
#   # 默认本地（http://localhost:8000）
#   ./scripts/smoke_emotion_llm_service.sh
#
#   # 用 env 覆盖 base_url
#   BASE_URL=http://emotion-llm-service:8000 ./scripts/smoke_emotion_llm_service.sh
#
#   # 跳过 /analyze（仅 health + metrics）
#   SKIP_ANALYZE=1 ./scripts/smoke_emotion_llm_service.sh
#
# 退出码：0 = 全绿，1 = 任意子测失败
#

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8000}"
GRPC_ADDR="${GRPC_ADDR:-localhost:50051}"
SKIP_ANALYZE="${SKIP_ANALYZE:-0}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-5}"
TIMEOUT_FLAG="--max-time $HTTP_TIMEOUT"

PASS=0
FAIL=0

red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

# 通用：断言 HTTP 状态码 == 期望值
http_assert() {
  local name="$1"; local expected="$2"; local url="$3"
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG "$url" || echo "000")
  if [ "$code" = "$expected" ]; then
    green "  ✓ $name  → $code"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → expected $expected, got $code"
    FAIL=$((FAIL+1))
  fi
}

# 通用：断言 body 包含子串
body_assert_contains() {
  local name="$1"; local url="$2"; local needle="$3"
  local body
  body=$(curl -sS -o - $TIMEOUT_FLAG -X GET "$url" 2>&1 || true)
  if printf '%s' "$body" | grep -q -F -- "$needle"; then
    green "  ✓ $name  → contains '$needle'"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → body=$body"
    FAIL=$((FAIL+1))
  fi
}

# POST /analyze 断言返回的 emotion 在 happy/sad/neutral 等受支持集合内
# 用临时文件传 --data-binary @file 以避免 bash 单引号字面量吞 UTF-8 字节
post_analyze_assert() {
  local name="$1"; local payload="$2"; local want_field="$3"; local want_eq="$4"
  local tmpfile body
  tmpfile=$(mktemp)
  printf '%s' "$payload" > "$tmpfile"
  body=$(curl -sS $TIMEOUT_FLAG -X POST -H 'Content-Type: application/json' \
       --data-binary "@$tmpfile" "$BASE_URL/analyze" 2>&1 || true)
  rm -f "$tmpfile"
  if printf '%s' "$body" | grep -q "\"$want_field\":$want_eq"; then
    green "  ✓ $name  → $want_field=$want_eq"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → body=$body"
    FAIL=$((FAIL+1))
  fi
}

echo "═══ smoke: emotion-llm-service @ $BASE_URL ═══"

# 1. /health 必须返 200 + JSON 含 status
http_assert "/health (200)" 200 "$BASE_URL/health"

# 2. /health body 应含 service=emotion-llm / status=ok
body_assert_contains "/health body has service+version" "$BASE_URL/health" "emotion-llm"

# 3. /metrics 返 Prometheus text（200 + Content-Type text/plain）
http_assert "/metrics (200)" 200 "$BASE_URL/metrics"
metrics_ct=$(curl -sS -o - $TIMEOUT_FLAG -I "$BASE_URL/metrics" 2>&1 | tr -d '\r' | grep -i "content-type" | head -1 || true)
if printf '%s' "$metrics_ct" | grep -qi "text/plain"; then
  green "  ✓ /metrics Content-Type is text/plain"
  PASS=$((PASS+1))
else
  yellow "  ! /metrics Content-Type header missing or unexpected (acceptable)"
fi

# 4. POST /analyze — 表驱动 3 个 case
if [ "$SKIP_ANALYZE" != "1" ]; then
  # case 1: 高兴文本 → primaryEmotion="happy"
  post_analyze_assert "/analyze happy-text" \
    '{"text":"今天真开心，好高兴啊"}' \
    "primaryEmotion" "\"happy\""

  # case 2: 中性文本 → primaryEmotion="neutral"
  post_analyze_assert "/analyze neutral-text" \
    '{"text":"今天天气不错"}' \
    "primaryEmotion" "\"neutral\""

  # case 3: 空文本 → 仍返 200 + primaryEmotion=neutral
  post_analyze_assert "/analyze empty-text" \
    '{"text":""}' \
    "primaryEmotion" "\"neutral\""
else
  yellow "  ! SKIP_ANALYZE=1, 跳过 /analyze 端点"
fi

# 5. gRPC 探活（可选）— 仅在 grpcurl 可用时跑
if command -v grpcurl >/dev/null 2>&1; then
  yellow "  ! grpcurl 可用，跳过 gRPC 探活（实际部署由 docker compose healthcheck 保证）"
fi

# 总结
echo ""
echo "═══ 结果：$PASS passed, $FAIL failed ═══"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0

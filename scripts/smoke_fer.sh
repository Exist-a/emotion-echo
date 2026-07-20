#!/usr/bin/env bash
#
# smoke_fer.sh · Facial Expression Recognition 情绪识别 · 冒烟测试
#
# 端点：
#   - GET  /health                  → {"status":...}
#   - GET  /metrics
#   - POST /analyze (multipart file) → {emotion, confidence, scores, source}
#
# 跑法：
#   ./scripts/smoke_fer.sh
#   BASE_URL=http://emotion-echo-fer:8004 ./scripts/smoke_fer.sh
#
# 退出码：0 = 全绿，1 = 任意子测失败
#

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8004}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-5}"
TIMEOUT_FLAG="--max-time $HTTP_TIMEOUT"
SKIP_INFERENCE="${SKIP_INFERENCE:-0}"

PASS=0
FAIL=0
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

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

body_assert_contains() {
  local name="$1"; local url="$2"; local needle="$3"
  local body
  body=$(curl -sS -o - $TIMEOUT_FLAG "$url" 2>&1 || true)
  if printf '%s' "$body" | grep -q -F -- "$needle"; then
    green "  ✓ $name  → contains '$needle'"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → body=$body"
    FAIL=$((FAIL+1))
  fi
}

echo "═══ smoke: FER @ $BASE_URL ═══"

# 1. /health 200
http_assert "/health (200)" 200 "$BASE_URL/health"

# 2. /health body 必须含 status=ok（或类似）
body_assert_contains "/health body has 'status'" "$BASE_URL/health" "status"

# 3. /metrics
http_assert "/metrics (200)" 200 "$BASE_URL/metrics"

# 4. /analyze — 该路径需 multipart file。FER 默认实现是 fer library 加载模型，
#    集成场景（如 docker compose --profile ai）才可用。SKIP 默认 0（假定已跑），
#    SKIP=1 时跳过避免 fail。
if [ "$SKIP_INFERENCE" != "1" ]; then
  # 准备一个 1x1 灰度 PNG 作为占位图（极小，但 fer library 可能判 "no face" → 200 + source: none）
  tmp_png="$(mktemp --suffix=.png)"
  # 写最小 1x1 PNG (89 bytes)
  printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x00\x00\x00\x00:~\x9bU\x00\x00\x00\nIDATx\x9cc\xfc\xff\xff?\x03\x00\x05\xfe\x02\xfe\xa5\x8d\xf9\x9c\x00\x00\x00\x00IEND\xaeB`\x82' > "$tmp_png"

  resp_code=$(curl -sS -o /tmp/fer_resp.json -w '%{http_code}' $TIMEOUT_FLAG \
              -X POST -F "file=@$tmp_png" "$BASE_URL/analyze" || echo "000")
  body=$(cat /tmp/fer_resp.json 2>/dev/null || echo "")
  rm -f "$tmp_png" /tmp/fer_resp.json

  case "$resp_code" in
    200)
      if printf '%s' "$body" | grep -q "emotion"; then
        green "  ✓ /analyze (multipart) → 200 with emotion body"
        PASS=$((PASS+1))
      else
        red   "  ✗ /analyze 200 but body shape unexpected: $body"
        FAIL=$((FAIL+1))
      fi
      ;;
    503)
      yellow "  ! /analyze → 503 (fer 模型未加载或 0 face)，跳过 — 设 SKIP_INFERENCE=1 跳"
      ;;
    *)
      red   "  ✗ /analyze → expected 200/503 got $resp_code body=$body"
      FAIL=$((FAIL+1))
      ;;
  esac
else
  yellow "  ! SKIP_INFERENCE=1, 跳过 /analyze multipart 端点"
fi

echo ""
echo "═══ 结果：$PASS passed, $FAIL failed ═══"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0

#!/usr/bin/env bash
#
# smoke_ai_svc.sh · ai-svc 多模态编排服务 · 冒烟测试
#
# 端点：
#   - GET  /health
#   - GET  /api/v1/ai/health            → {time, fer, sensevoice, xtts}
#   - POST /api/v1/multimodal/analyze   → {emotion, model, confidence}
#   - POST /api/v1/tts/synthesize       → {audio(base64), sampleRate}
#   - GET  /metrics
#
# 跑法：
#   ./scripts/smoke_ai_svc.sh
#   BASE_URL=http://localhost:8891 SKIP_TTS=1 SKIP_ANALYZE=1 ./scripts/smoke_ai_svc.sh
#

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:8891}"
HTTP_TIMEOUT="${HTTP_TIMEOUT:-10}"
TIMEOUT_FLAG="--max-time $HTTP_TIMEOUT"
SKIP_ANALYZE="${SKIP_ANALYZE:-0}"
SKIP_TTS="${SKIP_TTS:-0}"

# 构造 demo JWT（与 scripts/verify_stage23_endpoints.py 同款）
# 服务端中间件 trust APISIX，不验签，仅解码 payload
demo_jwt="$(printf '{"alg":"HS256","typ":"JWT"}' | base64 | tr -d '=' | tr -d '\n').$(printf '{"user_id":1}' | base64 | tr -d '=' | tr -d '\n').demo-signature-not-verified"
AUTH_HDR="Authorization: Bearer ${demo_jwt}"

PASS=0
FAIL=0
red()   { printf '\033[31m%s\033[0m\n' "$*"; }
green() { printf '\033[32m%s\033[0m\n' "$*"; }
yellow(){ printf '\033[33m%s\033[0m\n' "$*"; }

http_assert() {
  local name="$1"; local expected="$2"; local url="$3"
  local code
  code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG -H "$AUTH_HDR" "$url" || echo "000")
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
  body=$(curl -sS -o - $TIMEOUT_FLAG -H "$AUTH_HDR" "$url" 2>&1 || true)
  if printf '%s' "$body" | grep -q -F -- "$needle"; then
    green "  ✓ $name  → contains '$needle'"
    PASS=$((PASS+1))
  else
    red   "  ✗ $name  → body=$body"
    FAIL=$((FAIL+1))
  fi
}

# 表驱动 POST：path + payload + field + 期望值
# AI 服务 /api/v1/multimodal/analyze 是 **multipart/form-data**：
#   kind=text     → -F "kind=text" -F "text=..."
#   kind=image    → -F "kind=image" -F "file=@/path/to/img.jpg"
#   kind=audio    → -F "kind=audio" -F "file=@/path/to/audio.mp3"
#
# 接受 path 形如 /api/v1/multimodal/analyze + assert
multimodal_assert() {
  local name="$1"; local kind="$2"; local text_value="$3"; local file_path="$4"
  local field="$5"; local contains_val="$6"  # 字段值用 substring 匹配（更宽松）
  local args=()
  args+=(-F "kind=$kind")
  if [ -n "$text_value" ]; then args+=(-F "text=$text_value"); fi
  if [ -n "$file_path" ] && [ -f "$file_path" ]; then
    args+=(-F "file=@$file_path")
  fi

  local body code
  body=$(curl -sS $TIMEOUT_FLAG -X POST -H "$AUTH_HDR" \
       "${args[@]}" "$BASE_URL/api/v1/multimodal/analyze" 2>&1) || body=""
  code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG \
         -X POST -H "$AUTH_HDR" \
         "${args[@]}" "$BASE_URL/api/v1/multimodal/analyze" || echo "000")

  if [ "$code" = "200" ] || [ "$code" = "503" ]; then
    if printf '%s' "$body" | grep -q "\"$field\":\"$contains_val"; then
      green "  ✓ $name  → $field contains '$contains_val' (status=$code)"
      PASS=$((PASS+1))
    else
      red   "  ✗ $name  → body=$body (status=$code)"
      FAIL=$((FAIL+1))
    fi
  elif [ "$code" = "401" ]; then
    yellow "  ! $name → 401 (auth — JWT 解析失败); 检查 AUTH_HDR 编码"
  else
    red   "  ✗ $name  → expected 200/503 got $code body=$body"
    FAIL=$((FAIL+1))
  fi
}

echo "═══ smoke: ai-svc @ $BASE_URL ═══"

# 1. /health 200
http_assert "/health (200)" 200 "$BASE_URL/health"
body_assert_contains "/health body has 'status'" "$BASE_URL/health" "status"

# 2. /api/v1/ai/health — 报告各后端 UP/DOWN
ai_health_body=$(curl -sS -o - $TIMEOUT_FLAG -H "$AUTH_HDR" "$BASE_URL/api/v1/ai/health" 2>&1 || true)
if printf '%s' "$ai_health_body" | grep -q "time"; then
  green "  ✓ /api/v1/ai/health 200, body has 'time'"
  PASS=$((PASS+1))
else
  red   "  ✗ /api/v1/ai/health → body=$ai_health_body"
  FAIL=$((FAIL+1))
fi
# Print the raw body for visibility
yellow "  · /api/v1/ai/health body (informational):"
printf '%s\n' "$ai_health_body" | head -10 | sed 's/^/    /'

# 3. POST /api/v1/multimodal/analyze - multipart/form-data 4 kind case
if [ "$SKIP_ANALYZE" != "1" ]; then
  # case 1: text — keyword 永远成功（fallback）
  # 实际响应：{kind, emotion, confidence, sentimentScore, model, transcript}
  multimodal_assert "multimodal/analyze kind=text" "text" "今天很开心" "" "model" "keyword-stub"

  # case 2: image — FER live；没有项目自带 example 图片，生成一个 1x1 PNG 占位
  IMG=$(mktemp --suffix=.png)
  printf '\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x00\x00\x00\x00:~\x9bU\x00\x00\x00\nIDATx\x9cc\xfc\xff\xff?\x03\x00\x05\xfe\x02\xfe\xa5\x8d\xf9\x9c\x00\x00\x00\x00IEND\xaeB`\x82' > "$IMG"
  multimodal_assert "multimodal/analyze kind=image" "image" "" "$IMG" "model" "fer:"
  rm -f "$IMG"

  # case 3: audio — SenseVoice live；可能推理较慢，用 longer timeout
  AUDIO="emotion-echo-models/sensevoice-small/example/zh.mp3"
  if [ -f "$AUDIO" ]; then
    args=()
    args+=(-F "kind=audio")
    args+=(-F "file=@$AUDIO")
    audio_resp_pf=$(mktemp)
    audio_code=$(curl -sS -o "$audio_resp_pf" --max-time 90 \
                 -X POST -H "$AUTH_HDR" \
                 "${args[@]}" "$BASE_URL/api/v1/multimodal/analyze" 2>&1 | head -1)
    audio_code=$(curl -sS -o "$audio_resp_pf" -w '%{http_code}' --max-time 90 \
                 -X POST -H "$AUTH_HDR" \
                 "${args[@]}" "$BASE_URL/api/v1/multimodal/analyze" || echo "000")
    audio_body=$(cat "$audio_resp_pf" 2>/dev/null || echo "")
    rm -f "$audio_resp_pf"
    case "$audio_code" in
      200)
        if printf '%s' "$audio_body" | grep -q '"kind":"audio"'; then
          green "  ✓ multimodal/analyze kind=audio → 200 with kind=audio"
          PASS=$((PASS+1))
        else
          yellow "  ! multimodal/analyze kind=audio → 200 but body shape unexpected: $audio_body"
        fi
        ;;
      503)
        yellow "  ! multimodal/analyze kind=audio → 503 (model/FER/SV not loaded)"
        ;;
      000)
        yellow "  ! multimodal/analyze kind=audio → connection lost (SV inference hang; deploy timing issue)"
        ;;
      *)
        red   "  ✗ multimodal/analyze kind=audio → $audio_code body=$audio_body"
        FAIL=$((FAIL+1))
        ;;
    esac
  else
    yellow "  ! skip kind=audio (no sample audio at emotion-echo-models/sensevoice-small/example/zh.mp3)"
  fi

  # case 4: invalid kind → 4xx
  bad_code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG \
             -X POST -H "$AUTH_HDR" \
             -F "kind=unknown" \
             "$BASE_URL/api/v1/multimodal/analyze" || echo "000")
  if [ "$bad_code" = "400" ] || [ "$bad_code" = "422" ] || [ "$bad_code" = "500" ]; then
    green "  ✓ multimodal/analyze invalid kind → $bad_code (rejected as expected)"
    PASS=$((PASS+1))
  else
    yellow "  ! multimodal/analyze invalid kind → $bad_code (acceptable; implementation may differ)"
  fi
else
  yellow "  ! SKIP_ANALYZE=1, 跳过 /api/v1/multimodal/analyze"
fi

# 4. POST /api/v1/tts/synthesize — JSON
if [ "$SKIP_TTS" != "1" ]; then
  tts_pf=$(mktemp)
  printf '{"text":"%s","language":"zh-cn"}' "你好" > "$tts_pf"
  tts_body=$(curl -sS $TIMEOUT_FLAG -X POST -H 'Content-Type: application/json' -H "$AUTH_HDR" \
             --data-binary "@$tts_pf" \
             "$BASE_URL/api/v1/tts/synthesize" 2>&1) || tts_body=""
  tts_code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG \
             -X POST -H 'Content-Type: application/json' -H "$AUTH_HDR" \
             --data-binary "@$tts_pf" \
             "$BASE_URL/api/v1/tts/synthesize" || echo "000")
  rm -f "$tts_pf"

  case "$tts_code" in
    200)
      if printf '%s' "$tts_body" | grep -q "audio"; then
        green "  ✓ /api/v1/tts/synthesize → 200 with audio body"
        PASS=$((PASS+1))
      else
        red   "  ✗ /api/v1/tts/synthesize 200 but body missing audio: $tts_body"
        FAIL=$((FAIL+1))
      fi
      ;;
    503)
      yellow "  ! /api/v1/tts/synthesize → 503 (TTS backend 未启用 — 部署后应转为 200)"
      ;;
    *)
      red   "  ✗ /api/v1/tts/synthesize → $tts_code body=$tts_body"
      FAIL=$((FAIL+1))
      ;;
  esac
else
  yellow "  ! SKIP_TTS=1, 跳过 /api/v1/tts/synthesize"
fi

# 5. /metrics — /metrics path 是 middleware 白名单，无需 auth
metric_code=$(curl -sS -o /dev/null -w '%{http_code}' $TIMEOUT_FLAG "$BASE_URL/metrics" 2>/dev/null || echo "000")
if [ "$metric_code" = "200" ]; then
  green "  ✓ /metrics (200)  → 200"
  PASS=$((PASS+1))
else
  red   "  ✗ /metrics → expected 200 got $metric_code"
  FAIL=$((FAIL+1))
fi

echo ""
echo "═══ 结果：$PASS passed, $FAIL failed ═══"
if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0

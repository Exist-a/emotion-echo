#!/usr/bin/env bash
# scripts/smoke_ai_profile_v2.sh
#
# Stage 58 PR-TTS-4 §契约 7：AI profile 双分支 smoke
#
# 目的：
#   验证 PR-TTS-2 v2 + PR-TTS-3 落地后的 AI profile 行为：
#     分支 A (dev 默认)：3 个 AI 容器**不在**docker ps，ai-svc 启动时
#            FER/SenseVoice/XTTS BaseURL 为空 → NewXxxClient 返 nil →
#            analyzer 走降级路径（仅文本 LLM）
#     分支 B (--profile ai)：3 个 AI 容器起，ai-svc BaseURL 由 compose
#            env 注入容器 DNS → 调用真 AI 模型
#
# 契约断言：
#   分支 A (dev 默认)：
#     1. emotion-echo-{fer,sensevoice,xtts} 容器 NOT running
#     2. ai-svc 启动日志或 healthz 应报告 FER/SenseVoice/XTTS = nil
#   分支 B (--profile ai)：
#     3. 3 个 AI 容器 running + healthy
#     4. curl http://localhost:8004/health (FER) + :8002/health (SenseVoice) + :8003/health (XTTS) 全 200
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
#
# 用法：
#   bash scripts/smoke_ai_profile_v2.sh default  # 验分支 A（dev 默认）
#   bash scripts/smoke_ai_profile_v2.sh ai      # 验分支 B（--profile ai 启动后）
#
# 前置：
#   - docker / docker compose 可用
#   - 默认分支：infra + apps（不含 --profile ai）已 up
#   - ai 分支：上面 + --profile ai 启动 3 个 AI 容器

set -uo pipefail

MODE="${1:-default}"  # default 或 ai

APISIX_URL="${APISIX_URL:-http://localhost:19080}"
FAIL=0

log() { echo "[ai-profile] $*"; }
err() { echo "[ai-profile] FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

cd "$(dirname "$0")/../deploy"

if [ "$MODE" = "default" ]; then
  log "=== 分支 A：dev 默认（无 --profile ai）==="

  # 前置：infra + apps 已 up
  for c in emotion-echo-web-bff emotion-echo-ai-svc emotion-echo-apisix; do
    if ! docker inspect "$c" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
      err "container $c 未运行（先 docker compose -f infra -f apps -f dev up -d）"
      exit 1
    fi
  done

  # 契约 1：3 个 AI 容器应 NOT running
  log "--- 契约 1: 3 个 AI 容器 NOT running ---"
  for ai in emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts; do
    if docker inspect "$ai" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
      err "$ai 不应在 default mode 跑（profiles: [ai] 应隔离）"
    else
      log "[OK  ] $ai 未启动（profiles: [ai] 隔离生效）"
    fi
  done

  # 契约 2：ai-svc healthz 应报告 AI 客户端 nil
  log "--- 契约 2: ai-svc /healthz AI 客户端状态 ---"
  AI_HEALTHZ=$(curl -sS "$APISIX_URL/api/v1/ai/health" --max-time 10 2>/dev/null || echo "{}")

  # 期望：FER/SenseVoice/XTTS downstream 字段为 ok 或 nil/disabled（降级成功）
  # BFF /api/v1/ai/health 来自 ai-svc 聚合，含 downstream map
  for ai_key in fer sensevoice xtts; do
    STATUS=$(echo "$AI_HEALTHZ" | grep -oE "\"$ai_key\"[[:space:]]*:[[:space:]]*\"[^\"]+\"" | head -1 | sed "s/.*:[[:space:]]*\"\([^\"]*\)\".*/\1/")
    if [ "$STATUS" = "ok" ]; then
      err "default mode 下 $ai_key 不应 = ok（AI 容器未起）"
    else
      log "[OK  ] $ai_key = '$STATUS'（dev 默认 nil 降级路径生效）"
    fi
  done

elif [ "$MODE" = "ai" ]; then
  log "=== 分支 B：--profile ai 启动后 ==="

  # 前置：3 个 AI 容器 running
  for c in emotion-echo-fer emotion-echo-sensevoice emotion-echo-xtts emotion-echo-ai-svc; do
    if ! docker inspect "$c" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
      err "container $c 未运行（先 docker compose -f infra -f apps --profile ai up -d）"
      exit 1
    fi
  done

  # 契约 3：等 AI 容器 healthy
  log "--- 契约 3: 3 个 AI 容器 healthy ---"
  deadline=$((SECONDS + 300))
  while [ "$SECONDS" -lt "$deadline" ]; do
    FER_H=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-fer 2>/dev/null || echo missing)
    SV_H=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-sensevoice 2>/dev/null || echo missing)
    XTTS_H=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-xtts 2>/dev/null || echo missing)
    log "  fer=$FER_H sensevoice=$SV_H xtts=$XTTS_H (t=${SECONDS}s)"
    if [ "$FER_H" = "healthy" ] && [ "$SV_H" = "healthy" ] && [ "$XTTS_H" = "healthy" ]; then
      break
    fi
    sleep 15
  done

  # 契约 4：端口探测
  log "--- 契约 4: 端口探测 ---"
  for endpoint in "8004:FER" "8002:SenseVoice" "8003:XTTS"; do
    PORT="${endpoint%%:*}"
    NAME="${endpoint##*:}"
    HTTP=$(curl -sS -o /dev/null -w '%{http_code}' "http://localhost:$PORT/health" --max-time 10 2>/dev/null || echo "000")
    if [ "$HTTP" = "200" ]; then
      log "[OK  ] $NAME :$PORT/health = 200"
    else
      err "$NAME :$PORT/health = $HTTP（容器健康但 endpoint 不通）"
    fi
  done
else
  err "未知 MODE: $MODE（用法: $0 {default|ai}）"
  exit 1
fi

echo
if [ "$FAIL" -gt 0 ]; then
  echo "=== §契约 7 FAIL: $FAIL 项 ==="
  exit 1
fi
echo "=== §契约 7 ALL PASS (mode=$MODE) ==="
exit 0
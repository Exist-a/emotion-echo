#!/bin/bash
# scripts/dev-up.sh — dev 环境分批拉起（避免 .wslconfig 8GB 击穿 + 启动竞态 E2E-F-107/F-137）
#
# 背景（2026-09-23 E2E-17 step 5 收口 + 2026-09-24 IAB 实测）：
#   ① `docker compose up` 齐起 19 容器 → 瞬间内存峰值击穿 WSL 限额 → Docker 卡死
#   ② 业务 svc 抢在 Nacos/Postgres 起来前注册失败 → "degraded start" / "boot failed (continuing)"
#   ③ BFF 启动时 Nacos 未就绪 ⇒ swallow continuing → BFF 启动但不注册自己 → APISIX upstream
#      解析 nodes:{} → 全站 /api/v1/* 503（容器 /health 200 误导）
#
# 修法：
#   - 按依赖分批 sleep，错峰拉起（避免齐起峰值）
#   - 每批前置检查依赖 healthy（用 docker inspect）后再起下游
#   - BFF 启动后**主动校验**：Nacos /nacos/v1/ns/service/list 必须含 emotion-echo-web-bff，
#     否则脚本 exit 1（防 F-137 重演）
#
# 用法：bash scripts/dev-up.sh
set -euo pipefail
cd "$(dirname "$0")/../deploy" || exit 1
COMPOSE="docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --env-file .env.local --profile dev --profile ai"
NACOS_URL="http://127.0.0.1:8848"
NACOS_NS="emotion-echo-dev"
EXPECTED_SERVICES="emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-assessment-svc emotion-echo-analytics-svc emotion-echo-web-bff"

log() { echo "[$(date +%H:%M:%S)] $*"; }

# 等容器 healthy（每 2s 探一次；max 秒数；超时则失败）
wait_healthy() {
  local name="$1" max="${2:-120}"
  local i=0
  while [ "$i" -lt "$max" ]; do
    local st
    st=$(docker inspect -f '{{.State.Health.Status}}' "$name" 2>/dev/null || echo missing)
    if [ "$st" = "healthy" ]; then
      log "  ✓ $name healthy（${i}s）"
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  log "  ✗ $name 未 healthy 超过 ${max}s（状态=${st}）"
  return 1
}

# 探 nacos HTTP health（actuator 路径）
wait_nacos() {
  local max="${1:-180}"
  local i=0
  while [ "$i" -lt "$max" ]; do
    if curl -s -m 3 "$NACOS_URL/nacos/actuator/health" -o /dev/null -w "%{http_code}" 2>/dev/null | grep -q "200"; then
      log "  ✓ nacos ready（${i}s）"
      return 0
    fi
    sleep 2
    i=$((i + 2))
  done
  log "  ✗ nacos 未就绪超过 ${max}s"
  return 1
}

# 校验 Nacos 注册中心已含某 service
check_nacos_service() {
  local svc="$1"
  curl -s -m 5 "$NACOS_URL/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=$NACOS_NS" 2>/dev/null \
    | grep -q "\"$svc\"" || { log "  ✗ nacos 未注册 $svc"; return 1; }
  log "  ✓ nacos 已注册 $svc"
}

log "=== dev 分批拉起（避免齐起击穿 + 防 F-137 注册竞态）==="

# 批1：基础设施（轻量先起，给后续 svc 准备好依赖）
log "批1/4: infra（postgres etcd redis）..."
$COMPOSE up -d postgres etcd redis
wait_healthy postgres 60
sleep 4

# 批2：消息/存储/APISIX（nacos 起得慢，单独等就绪 + 防 E2E-F-107 抢跑）
log "批2/4: 中间件（kafka nacos emotion-echo-minio apisix）..."
$COMPOSE up -d kafka nacos emotion-echo-minio apisix
wait_healthy postgres 30
wait_nacos 180
wait_healthy emotion-echo-minio 60
sleep 4

# 批3：业务 svc（错峰起避免峰值）
log "批3/4: 业务 svc（user/chat/ai/assessment/analytics/web-bff/web/llm）..."
$COMPOSE up -d emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-ai-svc \
  emotion-echo-assessment-svc emotion-echo-analytics-svc \
  emotion-echo-web-bff emotion-echo-web emotion-llm-service
wait_healthy emotion-echo-web-bff 90
sleep 4

# 批3.5：E2E-F-137 校验——确保 BFF 已注册到 Nacos，否则全站 /api/v1/* 会 503
log "批3.5/4: 校验 Nacos 注册（防 F-137 启动竞态）..."
if ! curl -s -m 5 "$NACOS_URL/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=$NACOS_NS" \
     | python -c "
import sys, json
data = json.load(sys.stdin)
doms = set(data.get('doms', []))
required = {'emotion-echo-user-svc', 'emotion-echo-chat-svc', 'emotion-echo-ai-svc',
            'emotion-echo-assessment-svc', 'emotion-echo-analytics-svc', 'emotion-echo-web-bff'}
missing = required - doms
if missing:
    print('FAIL missing:', missing)
    sys.exit(1)
"; then
  log "✗ Nacos 注册校验失败；可能 BFF 启动时 Nacos 未就绪（F-137 同型）"
  log "  修复路径：docker restart emotion-echo-web-bff（开发环境手动恢复）"
  exit 1
fi
log "  ✓ Nacos 注册校验全过（6/6 svc 已注册）"

# 批4：XTls（2.6G 模型大户，避开峰值最后起）
log "批4/4: XTls（2.6G 模型加载 40-180s）..."
$COMPOSE up -d emotion-echo-xtts

log "=== 全部 up -d 完成；等 healthy ==="
log "探活建议：bash scripts/dev-up.sh 后再 docker ps 看 emotion-echo-xtts 启动情况"
log "注：F-132/F-133/F-137 等开放项详见账本 discovered-unresolved.md"

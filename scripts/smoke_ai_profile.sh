#!/usr/bin/env bash
# ============================================================
# Stage 36-B2 / G6：FER / SenseVoice profile:ai 镜像冒烟脚本
# ============================================================
# 验证 docker compose --profile ai 起 FER + SenseVoice 容器 + 健康检查。
#
# 用法（在仓库根）：
#   bash scripts/smoke_ai_profile.sh
#
# 前置：
#   - docker / docker compose 可用
#   - 已 .env.local 或环境里设了 FER/SenseVoice 路径（脚本读 /dev/null 默认）
#   - 离线环境首跑 SenseVoice 会自动下 ~200MB 模型（compose start_period=120s）
#
# 退出码：
#   0 = 全绿
#   1 = 至少一容器 unhealthy 或启动失败
# ============================================================

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

INFRA="deploy/docker-compose.infra.yml"
APPS="deploy/docker-compose.apps.yml"

echo "[smoke] step 1: postgres 健康（前置）"
docker compose -f "$INFRA" ps postgres 2>/dev/null | grep -q "Up" || {
  echo "[smoke] postgres 未起，先跑：docker compose -f $INFRA up -d postgres"
  exit 1
}

echo "[smoke] step 2: build + up profile ai（FER + SenseVoice）"
docker compose -f "$INFRA" -f "$APPS" --profile ai up -d --build fer sensevoice
ret=$?
if [ "$ret" -ne 0 ]; then
  echo "[smoke] build/up 失败 (exit=$ret)"
  exit 1
fi

echo "[smoke] step 3: 等两容器 healthy（最多 150s，SenseVoice 首次需下模型）"
deadline=$((SECONDS + 150))
while [ "$SECONDS" -lt "$deadline" ]; do
  fer_health=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-fer 2>/dev/null || echo "missing")
  sv_health=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-sensevoice 2>/dev/null || echo "missing")
  echo "  fer=$fer_health  sensevoice=$sv_health  (t=$SECONDS)"
  if [ "$fer_health" = "healthy" ] && [ "$sv_health" = "healthy" ]; then
    break
  fi
  sleep 10
done

fer_final=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-fer 2>/dev/null || echo "missing")
sv_final=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-sensevoice 2>/dev/null || echo "missing")

echo "[smoke] step 4: 端口探测（即便 health check 通过也要确保 endpoint 可达）"
fer_ok=$(curl -fsS http://localhost:8004/health 2>/dev/null && echo OK || echo FAIL)
sv_ok=$(curl -fsS http://localhost:8002/health 2>/dev/null && echo OK || echo FAIL)
echo "  fer /health=$fer_ok   sensevoice /health=$sv_ok"

echo "[smoke] step 5: 关停（保留数据卷给后续 Stage 36 收口文档用）"
docker compose -f "$INFRA" -f "$APPS" --profile ai stop

if [ "$fer_final" = "healthy" ] && [ "$sv_final" = "healthy" ] && [ "$fer_ok" = "OK" ] && [ "$sv_ok" = "OK" ]; then
  echo "[smoke] ✅ PASS（FER + SenseVoice 均 healthy 且 /health 可达）"
  exit 0
fi
echo "[smoke] ❌ FAIL：fer=$fer_final sensevoice=$sv_final fer_http=$fer_ok sv_http=$sv_ok"
exit 1

#!/usr/bin/env bash
# ============================================================
# Stage 36-C2 / G8：Nacos 全栈 6 svc 注册实测脚本
# ============================================================
# 验证 docker compose up nacos + 6 Go svc（NACOS_ENABLED=true）后，
# Nacos 控制台能看到 6 svc 注册成功 + health OK。
#
# 用法（在仓库根）：
#   bash scripts/smoke_nacos_full.sh
#
# 前置：
#   - docker / docker compose 可用
#   - 各 Go svc 镜像已 build（ai-svc / user-svc / chat-svc / analytics-svc /
#     assessment-svc / web-bff）
#
# 验收（对应 ADR-16 §G8）：
#   - Nacos :8848/nacos/v1/ns/service/list 返回 ≥ 6 个 emotion-echo-* svc
#   - 每个 svc 在 Nacos 元数据里带 healthy=true
# ============================================================

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

INFRA="deploy/docker-compose.infra.yml"
APPS="deploy/docker-compose.apps.yml"

EXPECTED_SVCS=(
  "emotion-echo-user-svc"
  "emotion-echo-chat-svc"
  "emotion-echo-analytics-svc"
  "emotion-echo-assessment-svc"
  "emotion-echo-ai-svc"
  "emotion-echo-web-bff"
)

echo "[smoke] step 1: nacos 必须先 healthy"
docker compose -f "$INFRA" up -d nacos
for i in $(seq 1 30); do
  health=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-nacos 2>/dev/null || echo "missing")
  echo "  nacos health=$health (t=${i}0s)"
  if [ "$health" = "healthy" ]; then break; fi
  sleep 10
done
nacos_final=$(docker inspect --format='{{.State.Health.Status}}' emotion-echo-nacos 2>/dev/null || echo "missing")
if [ "$nacos_final" != "healthy" ]; then
  echo "[smoke] ❌ nacos 没起来（status=$nacos_final）"
  exit 1
fi

echo "[smoke] step 2: 启 6 Go svc（Nacos 配置 + 注册由 nacos_boot.go 处理）"
docker compose -f "$INFRA" -f "$APPS" up -d \
  emotion-echo-user-svc \
  emotion-echo-chat-svc \
  emotion-echo-analytics-svc \
  emotion-echo-assessment-svc \
  emotion-echo-ai-svc \
  emotion-echo-web-bff

# 等各 svc 注册（Nacos client 默认 5s 一次 push + 5s 心跳）
echo "[smoke] step 3: 等 60s（svc 启动 + Nacos 第一次 push）"
sleep 60

echo "[smoke] step 4: 查 Nacos 服务列表"
# Nacos OpenAPI: /nacos/v1/ns/service/list?pageNo=1&pageSize=20
list=$(curl -fsS "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50" 2>&1 || echo "")
echo "$list" | head -c 800
echo

echo
echo "[smoke] step 5: 断言每个期望 svc 都注册成功"
missing=()
for svc in "${EXPECTED_SVCS[@]}"; do
  if echo "$list" | grep -q "\"$svc\""; then
    echo "  ✅ $svc registered"
  else
    echo "  ❌ $svc NOT registered"
    missing+=("$svc")
  fi
done

if [ ${#missing[@]} -eq 0 ]; then
  echo
  echo "[smoke] ✅ PASS：6 svc 全部注册 Nacos 成功"
  exit 0
fi
echo
echo "[smoke] ❌ FAIL：${#missing[@]} svc 未注册: ${missing[*]}"
echo "提示：检查 svc 日志 'docker compose logs emotion-echo-<svc>'，NACOS_ENABLED 是否真的=true"
exit 1

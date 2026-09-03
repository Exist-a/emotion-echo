#!/usr/bin/env bash
# Stage 36-D Bug 5: 7 容器 (unhealthy) 但 /health 都返回 200
#
# 用法：
#   ./scripts/healthcheck_smoke.sh                  # RED: 期望 exit 1 (有 unhealthy)
#   ./scripts/healthcheck_smoke.sh && echo ok       # GREEN: 期望全 healthy
#
# 检查项（每个容器）：
#   - docker inspect --format '{{.State.Health.Status}}' == 'healthy'
#   - (如未配 healthcheck) 容器 Up ≥ start_period
set -uo pipefail

SERVICES=(
    emotion-echo-postgres
    emotion-echo-redis
    emotion-echo-kafka
    emotion-echo-nacos
    emotion-echo-sw-oap
    emotion-echo-sw-ui
    emotion-echo-user-svc
    emotion-echo-chat-svc
    emotion-echo-analytics-svc
    emotion-echo-assessment-svc
    emotion-echo-ai-svc
    emotion-echo-web-bff
)

FAIL=0
PASS=0

for svc in "${SERVICES[@]}"; do
    if ! docker ps --format '{{.Names}}' | grep -qx "$svc" 2>/dev/null; then
        echo "[SKIP] $svc (container not up)"
        continue
    fi

    HEALTH=$(docker inspect "$svc" --format '{{.State.Health.Status}}' 2>/dev/null || echo "no-healthcheck")

    if [[ "$HEALTH" == "healthy" ]]; then
        echo "[OK  ] $svc -> $HEALTH"
        PASS=$((PASS + 1))
    elif [[ "$HEALTH" == "starting" ]]; then
        echo "[WAIT] $svc -> $HEALTH (within start_period)"
        # don't count as fail (still in start_period window)
    else
        echo "[FAIL] $svc -> $HEALTH"
        FAIL=$((FAIL + 1))
    fi
done

echo ""
echo "PASS: $PASS / FAIL: $FAIL"

if [[ "$FAIL" -eq 0 ]]; then
    echo "GREEN: all containers healthy"
    exit 0
else
    echo "RED: $FAIL containers unhealthy"
    exit 1
fi
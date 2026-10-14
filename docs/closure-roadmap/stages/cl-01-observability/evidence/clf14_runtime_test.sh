#!/bin/bash
# CL-F-14 运行时验证（负向对照 + 修复验证）
#
# 流程：
#   A 负向对照：停 OAP → 用 cl01-verify 镜像重建 chat-svc（启动期 tracer 必失败）
#              → 断言 SkyWalking 服务列表里**没有** chat-svc
#   B 修复验证：起 OAP → **不重启 chat-svc** → 等后台补挂 → 断言 chat-svc **自动出现**
#
# 关键：B 步不许重启 chat-svc —— 这才证明"后台补挂 + 入口路径现读"真的生效，
#       而不是靠编排层重启兜底。
set -uo pipefail

cd /d/源码/Emotion-Echo-cl01/deploy || exit 1
SWPY=/d/源码/Emotion-Echo-cl01/docs/closure-roadmap/stages/cl-01-observability/evidence/_sw_services.py
COMPOSE="docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml -f docker-compose.cl01-verify.yml --env-file .env.local --profile dev --profile obs"

echo "########## 阶段 A：负向对照（OAP 停着重建 chat-svc）##########"
echo "--- A1 停 OAP ---"
docker stop emotion-echo-sw-oap >/dev/null 2>&1
docker stop emotion-echo-sw-ui  >/dev/null 2>&1
sleep 5
echo "oap_state=$(docker inspect -f '{{.State.Status}}' emotion-echo-sw-oap 2>/dev/null)"

echo "--- A2 用 cl01-verify 镜像重建 chat-svc（OAP 不可达 ⇒ 启动期 tracer 初始化必失败）---"
$COMPOSE up -d --force-recreate --no-deps emotion-echo-chat-svc 2>&1 | tail -3
echo "--- 等 40s 让启动期重试（5×2s）走完 ---"
sleep 40

echo "--- A3 chat-svc 启动日志（应看到 tracer init failed）---"
docker logs emotion-echo-chat-svc --since 2m 2>&1 | grep -iE "skywalking|tracer" | tail -6

echo "--- A4 断言：SkyWalking 服务列表里【没有】chat-svc ---"
python "$SWPY" chat-svc; A_RC=$?
echo "A4_exit=$A_RC  (期望 1 = ABSENT)"

echo ""
echo "########## 阶段 B：修复验证（起 OAP，不重启 chat-svc）##########"
echo "--- B1 起 OAP ---"
docker start emotion-echo-sw-oap >/dev/null 2>&1
docker start emotion-echo-sw-ui  >/dev/null 2>&1
echo "--- 等 OAP 就绪 60s ---"
sleep 60

echo "--- B2 轮询 SkyWalking 服务列表（最多 5 分钟，退避 5s/10s/20s/40s/60s）---"
B_RC=1
for i in $(seq 1 12); do
  sleep 25
  python "$SWPY" chat-svc > /tmp/swpoll.txt 2>&1
  rc=$?
  echo "[poll $i] rc=$rc :: $(tail -2 /tmp/swpoll.txt | head -1)"
  if [ $rc -eq 0 ]; then B_RC=0; break; fi
done
echo "B2_exit=$B_RC  (期望 0 = FOUND)"

echo "--- B3 chat-svc 日志：后台补挂成功 ---"
docker logs emotion-echo-chat-svc --since 10m 2>&1 | grep -iE "后台补挂" | tail -5

echo ""
echo "########## 结论 ##########"
echo "A(负向对照,期望 ABSENT=1): $A_RC"
echo "B(修复验证,期望 FOUND=0): $B_RC"
if [ "$A_RC" -eq 1 ] && [ "$B_RC" -eq 0 ]; then
  echo "RESULT: PASS —— 修复生效（OAP 恢复后 chat-svc 未重启即自动出现在 SkyWalking）"
else
  echo "RESULT: NOT PASS —— 见上面日志"
fi

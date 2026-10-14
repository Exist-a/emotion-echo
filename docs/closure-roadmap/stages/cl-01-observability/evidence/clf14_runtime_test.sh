#!/bin/bash
# CL-F-14 运行时验证（**修正版**：不再停 sw-ui —— 它是 GraphQL 查询代理，
#   停了会让断言本身查不通。2026-10-14 首版就踩了这个坑，导致证据文件自述
#   "QUERY_FAILED / A4_exit=2 / RESULT: NOT PASS" 而被误当 PASS 引用）。
#
# 流程：
#   A 造条件：停 OAP → 用 cl01-verify 镜像重建 chat-svc（启动期 tracer 必失败）
#            → 断言日志里出现 "tracer init failed" + "后台补挂第 N 次失败"（证明进了补挂路径）
#   B 验证  ：起 OAP → **不重启 chat-svc** → 轮询 SkyWalking 服务列表 → 断言出现 chat-svc
#
# 关键：B 步不许重启 chat-svc —— 这才证明"后台补挂 + 入口路径现读"真生效，
#       而不是靠编排层重启兜底（D-07 刻意排除的替代方案）。
set -uo pipefail

cd /d/源码/Emotion-Echo-cl01/deploy || exit 1
EV=/d/源码/Emotion-Echo-cl01/docs/closure-roadmap/stages/cl-01-observability/evidence
SWPY=$EV/clf14_sw_services.py
COMPOSE="docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml -f docker-compose.cl01-verify.yml --env-file .env.local --profile dev --profile obs"

echo "########## 阶段 A：造条件（停 OAP 重建 chat-svc；**sw-ui 保持运行**）##########"
echo "--- A1 只停 OAP ---"
docker stop emotion-echo-sw-oap >/dev/null 2>&1
sleep 5
echo "oap_state=$(docker inspect -f '{{.State.Status}}' emotion-echo-sw-oap 2>/dev/null)"
echo "sw_ui_state=$(docker inspect -f '{{.State.Status}}' emotion-echo-sw-ui 2>/dev/null)"

echo "--- A2 用 cl01-verify 镜像重建 chat-svc ---"
$COMPOSE up -d --force-recreate --no-deps emotion-echo-chat-svc 2>&1 | tail -3
docker inspect -f 'image={{.Config.Image}}' emotion-echo-chat-svc
echo "--- 等 45s 让启动期重试（5×2s）走完并进入后台补挂 ---"
sleep 45

echo "--- A3 断言：日志出现 init failed + 后台补挂失败行 ---"
A_LOG=$(docker logs emotion-echo-chat-svc --since 3m 2>&1 | grep -cE "tracer init failed|后台补挂第 [0-9]+ 次失败")
echo "A3_matching_lines=$A_LOG  (期望 >=2)"
docker logs emotion-echo-chat-svc --since 3m 2>&1 | grep -iE "skywalking" | tail -4

echo ""
echo "########## 阶段 B：验证（起 OAP，不重启 chat-svc）##########"
echo "--- B1 起 OAP ---"
docker start emotion-echo-sw-oap >/dev/null 2>&1
echo "--- 等 OAP 就绪 60s ---"
sleep 60

echo "--- B2 轮询 SkyWalking 服务列表（最多 5 分钟；后台退避 5/10/20/40/60s）---"
B_RC=1
for i in $(seq 1 12); do
  sleep 25
  python "$SWPY" chat-svc > /tmp/swpoll.txt 2>&1
  rc=$?
  echo "[poll $i] rc=$rc :: $(tail -2 /tmp/swpoll.txt | head -1)"
  if [ $rc -eq 0 ]; then B_RC=0; break; fi
done
echo "B2_exit=$B_RC  (期望 0 = FOUND)"

echo "--- B3 日志：后台补挂成功 ---"
B_OK=$(docker logs emotion-echo-chat-svc --since 15m 2>&1 | grep -c "后台补挂成功")
echo "B3_成功行数=$B_OK  (期望 >=1)"
docker logs emotion-echo-chat-svc --since 15m 2>&1 | grep "后台补挂成功" | tail -2

echo ""
echo "########## 结论 ##########"
echo "A3(进入补挂路径, 期望 >=2): $A_LOG"
echo "B2(自动回归 SkyWalking, 期望 0): $B_RC"
echo "B3(补挂成功日志, 期望 >=1): $B_OK"
if [ "$A_LOG" -ge 2 ] && [ "$B_RC" -eq 0 ] && [ "$B_OK" -ge 1 ]; then
  echo "RESULT: PASS —— OAP 恢复后 chat-svc 未重启即自动出现在 SkyWalking"
else
  echo "RESULT: NOT PASS —— 见上面日志"
fi

#!/bin/bash
# CL-F-14 负向对照：用【旧镜像 v0.1.16（无修复）】跑同一场景
# 期望：OAP 恢复后 chat-svc **仍然缺席**（旧代码启动期失败后不再重试）
set -uo pipefail
cd /d/源码/Emotion-Echo-cl01/deploy || exit 1
SWPY=/d/源码/Emotion-Echo-cl01/docs/closure-roadmap/stages/cl-01-observability/evidence/_sw_services.py
# 注意：**不带** cl01-verify override ⇒ 用 compose 里的 emotion-echo/chat-svc:v0.1.16
COMPOSE_OLD="docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev --profile obs"

echo "########## 负向对照：旧镜像 v0.1.16（无 CL-F-14 修复）##########"
echo "--- 停 OAP（保留 sw-ui 以便之后查询）---"
docker stop emotion-echo-sw-oap >/dev/null 2>&1
sleep 5

echo "--- 用旧镜像重建 chat-svc ---"
$COMPOSE_OLD up -d --force-recreate --no-deps emotion-echo-chat-svc 2>&1 | tail -3
docker inspect -f 'image={{.Config.Image}}' emotion-echo-chat-svc
sleep 45

echo "--- 旧镜像的启动日志（应只有 init failed，**没有**后台补挂行）---"
docker logs emotion-echo-chat-svc --since 2m 2>&1 | grep -iE "skywalking|tracer|后台补挂" | tail -5

echo "--- 起 OAP，等 60s ---"
docker start emotion-echo-sw-oap >/dev/null 2>&1
sleep 60

echo "--- 轮询 5 分钟：旧镜像下 chat-svc 应【始终缺席】---"
RC=1
for i in $(seq 1 12); do
  sleep 25
  python "$SWPY" chat-svc > /tmp/swn.txt 2>&1
  rc=$?
  echo "[poll $i] rc=$rc :: $(tail -2 /tmp/swn.txt | head -1)"
  if [ $rc -eq 0 ]; then RC=0; break; fi
done
echo "negative_exit=$RC  (期望 1 = 始终 ABSENT)"
echo "--- 旧镜像 10 分钟日志里有没有'后台补挂'字样 ---"
docker logs emotion-echo-chat-svc --since 10m 2>&1 | grep -c "后台补挂"
echo "RESULT: $([ "$RC" -eq 1 ] && echo 'NEGATIVE-CONTROL OK（旧镜像确实不恢复）' || echo 'NEGATIVE-CONTROL FAIL（旧镜像也恢复了 ⇒ 对照无效）')"

#!/usr/bin/env bash
# CL-02 起栈验证：用本轨 :cl02-verify 镜像重建 6 个服务（H5/条款10：不整栈 up、不覆盖公共 tag）
set -u
SHARED="D:/源码/Emotion-Echo"
WT="D:/源码/Emotion-Echo-cl02"
cd "$SHARED/deploy" || exit 2
COMPOSE="docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml -f $WT/deploy/docker-compose.cl02-verify.yml --env-file .env.local"
SVCS="emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc emotion-echo-ai-svc emotion-echo-web-bff emotion-llm-service"

echo "===== 1) 用 cl02 镜像重建 6 个服务（--no-deps，不整栈） ====="
$COMPOSE up -d --no-deps $SVCS 2>&1 | tail -12

echo
echo "===== 2) 等 healthy（≤150s） ====="
for i in $(seq 1 30); do
  st=$(docker ps --format '{{.Names}}|{{.Status}}' | grep -E "emotion-echo-(user-svc|chat-svc|analytics-svc|ai-svc|web-bff)\||emotion-llm-service" | grep -c healthy)
  echo "  healthy=$st/6  ($(date +%H:%M:%S))"
  [ "$st" = "6" ] && break
  sleep 5
done

echo
echo "===== 3) H5 复核：容器跑的镜像是否为本轨 :cl02-verify ====="
docker ps --format '{{.Names}}|{{.Image}}' | grep -E "user-svc|chat-svc|analytics-svc|ai-svc|web-bff|llm-service" | sort

echo
echo "===== 4) 重建 svc 后重跑 apisix-seed（铁律） ====="
$COMPOSE up -d --force-recreate --no-deps emotion-echo-apisix-seed 2>&1 | tail -3
sleep 20
docker logs emotion-echo-apisix-seed 2>&1 | tail -6

echo
echo "===== 5) T-21：Nacos 服务注册（期望 count:6） ====="
curl -s "http://localhost:8848/nacos/v1/ns/service/list?pageNo=1&pageSize=50&namespaceId=emotion-echo-dev"
echo

echo
echo "===== 6) T-15 运行时：*_TRANSPORT env（空=未设 ⇒ 走代码默认 grpc） ====="
for c in emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc emotion-echo-ai-svc emotion-echo-web-bff; do
  printf "%-28s " "$c"
  docker inspect "$c" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep -E "_TRANSPORT=" | tr '\n' ' '
  echo
done

echo
echo "===== 7) 网关可达性（走 APISIX :19080） ====="
curl -s -o /dev/null -w "login HTTP %{http_code}\n" -X POST "http://localhost:19080/api/v1/auth/login" \
  -H "Content-Type: application/json" -d '{"username":"echo","password":"echo123"}'

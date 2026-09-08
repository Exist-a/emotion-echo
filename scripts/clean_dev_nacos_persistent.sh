#!/usr/bin/env bash
# scripts/clean_dev_nacos_persistent.sh — Stage-52 运维清理
#
# 背景：PR-1 把 defaultRegisterEphemeral 改成 false，5 个 dev svc 的
# serviceName 被创建为 persistent。Stage-52 还原为 ephemeral=true，
# 但 Nacos Derby 里残留 6 个 persistent serviceName 会导致 SDK 重启
# 时仍以 ephemeral 注册 → 400 失败。
#
# 本脚本：
#   1. 列出 emotion-echo-dev namespace 下所有 service
#   2. 逐一删（先 instance 后 service）
#   3. 让 5 svc 重启后 SDK 自动按 ephemeral=true 重建
#
# 必须在 **停掉 6 个业务 svc 之后** 运行（避免 svc 持续注册导致 service 删不干净）。
#
# 用法（在 host 端运行）：
#   docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
#     stop emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
#     emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff
#   bash scripts/clean_dev_nacos_persistent.sh
#   docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
#     start emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
#     emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff
#
# 实现：从 host 调用 docker exec 进 nacos 容器内 curl 访问 localhost，
# 绕过 Windows WSL2 host loopback 端口转发的怪异问题（host curl 直连
# :8848 返 52 Empty reply, docker exec 内 curl 正常）。
# JSON 解析用 grep/sed/awk，避免依赖 jq/python。
#
# 仅 dev 环境使用。prod 集群禁用此脚本——Nacos 集群模式下 service 删错影响扩缩容。

set -euo pipefail

NACOS_CONTAINER="${NACOS_CONTAINER:-emotion-echo-nacos}"
NACOS_PORT="${NACOS_PORT:-8848}"
NAMESPACE="${NAMESPACE:-emotion-echo-dev}"
GROUP="${GROUP:-DEFAULT_GROUP}"

nacos_curl() {
  docker exec "$NACOS_CONTAINER" curl -fsS "$@"
}

# 列 serviceName
echo "==> list services in namespace=$NAMESPACE group=$GROUP"
SERVICES_JSON=$(nacos_curl "http://localhost:${NACOS_PORT}/nacos/v1/ns/service/list?pageNo=1&pageSize=100&namespaceId=${NAMESPACE}")
SVCS=$(echo "$SERVICES_JSON" | grep -oE '"emotion-echo-[a-z-]+"' | sort -u | tr -d '"' || true)

if [ -z "$SVCS" ]; then
  echo "no emotion-echo-* service found, exit"
  exit 0
fi

echo "found services:"
echo "$SVCS" | sed 's/^/  /'

echo ""
echo "==> deleting each service (instances first, then service)..."
for svc in $SVCS; do
  INST_JSON=$(nacos_curl "http://localhost:${NACOS_PORT}/nacos/v1/ns/instance/list?serviceName=${svc}&namespaceId=${NAMESPACE}&groupName=${GROUP}")

  # 提取 instanceId（格式 "ip#port#cluster#group@@serviceName"），逐个删 instance
  # 用 tr 把逗号变 newline + grep 取 instanceId 字段
  echo "$INST_JSON" | tr ',' '\n' | grep -oE '"instanceId":"[^"]+"' | sed 's/.*"instanceId":"//;s/"$//' > /tmp/_ids.$$
  while read -r instid; do
    [ -z "$instid" ] && continue
    # instid 形如 "172.18.0.15#8891#DEFAULT#DEFAULT_GROUP@@emotion-echo-user-svc"
    ip=$(echo "$instid" | awk -F'#' '{print $1}')
    port=$(echo "$instid" | awk -F'#' '{print $2}')
    if [ -n "$ip" ] && [ -n "$port" ]; then
      echo "  DELETE instance $svc $ip:$port"
      docker exec "$NACOS_CONTAINER" curl -fsS -X DELETE \
        "http://localhost:${NACOS_PORT}/nacos/v1/ns/instance?serviceName=${svc}&ip=${ip}&port=${port}&namespaceId=${NAMESPACE}&groupName=${GROUP}" \
        >/dev/null || true
    fi
  done < /tmp/_ids.$$
  rm -f /tmp/_ids.$$

  echo "  DELETE service $svc"
  docker exec "$NACOS_CONTAINER" curl -fsS -X DELETE \
    "http://localhost:${NACOS_PORT}/nacos/v1/ns/service?serviceName=${svc}&namespaceId=${NAMESPACE}&groupName=${GROUP}" \
    >/dev/null
done

echo ""
echo "==> verify (list should be empty)"
nacos_curl "http://localhost:${NACOS_PORT}/nacos/v1/ns/service/list?pageNo=1&pageSize=100&namespaceId=${NAMESPACE}"
echo ""

echo ""
echo "==> done. Now restart the 6 dev svc to let SDK re-register as ephemeral:"
echo "    docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml start emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff"
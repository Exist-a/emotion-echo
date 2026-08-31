#!/usr/bin/env bash
#
# list_nacos_instances.sh — Stage 31 PR-11
# 列出 Nacos 中已注册的 service-name 实例（健康状态）。
#
# 用法：
#   ./scripts/list_nacos_instances.sh
#   NACOS_ADDR=emotion-echo-nacos:8848 ./scripts/list_nacos_instances.sh
#
# 默认 namespace=emotion-echo-dev；可在第一个参数覆盖。

set -euo pipefail

NAMESPACE="${1:-${NACOS_NAMESPACE:-emotion-echo-dev}}"
NACOS_ADDR="${NACOS_ADDR:-localhost:8848}"
NAMESPACE_ID=$(echo -n "${NAMESPACE}" | md5sum | cut -c1-32 || echo "${NAMESPACE}")

echo "==> Nacos 服务实例列表（namespace=${NAMESPACE}）"
echo

# 取全部 service 列表
SERVICES=$(curl -sS "http://${NACOS_ADDR}/nacos/v1/ns/serviceList?pageNo=1&pageSize=100&namespaceId=${NAMESPACE_ID}" \
    | jq -r '.doms[]?' 2>/dev/null || true)

if [ -z "${SERVICES}" ]; then
    echo "    （暂无注册服务；请先启动 compose + 各 svc）"
    exit 0
fi

for svc in ${SERVICES}; do
    echo "---- ${svc} ----"
    INSTANCES=$(curl -sS "http://${NACOS_ADDR}/nacos/v1/ns/instance/list?serviceName=${svc}&namespaceId=${NAMESPACE_ID}" || true)
    echo "${INSTANCES}" | jq -r '.hosts[]? | "  ip=\(.ip) port=\(.port) healthy=\(.healthy) enabled=\(.enabled) metadata=\(.metadata)"' 2>/dev/null || \
        echo "  (raw) ${INSTANCES}"
done

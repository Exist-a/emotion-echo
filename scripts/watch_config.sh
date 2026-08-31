#!/usr/bin/env bash
#
# watch_config.sh — Stage 31 PR-11
# 监听 Nacos 中指定 dataId 的配置变更（长轮询；30s 超时）。
#
# 用法：
#   ./scripts/watch_config.sh user-svc.ops.yaml
#   ./scripts/watch_config.sh ai-svc.ops.yaml DEFAULT_GROUP
#
# 默认 namespace=emotion-echo-dev；可在 env 覆盖。

set -euo pipefail

DATA_ID="${1:?用法: $0 <dataId> [group]}"
GROUP="${2:-${GROUP:-DEFAULT_GROUP}}"
NAMESPACE="${NACOS_NAMESPACE:-emotion-echo-dev}"
NACOS_ADDR="${NACOS_ADDR:-localhost:8848}"
NAMESPACE_ID=$(echo -n "${NAMESPACE}" | md5sum | cut -c1-32 || echo "${NAMESPACE}")

echo "==> 监听 Nacos 配置变更"
echo "    dataId    = ${DATA_ID}"
echo "    group     = ${GROUP}"
echo "    namespace = ${NAMESPACE}"
echo "    按 Ctrl+C 退出"
echo

# 长轮询：/nacos/v1/cs/configs/listener 接受 ?dataId&group&content&tenant
# 简化：每 3s 重新 GET 全量，比较 md5 并打印差异
LAST_CONTENT=""
while true; do
    CONTENT=$(curl -sS "http://${NACOS_ADDR}/nacos/v1/cs/configs?dataId=${DATA_ID}&group=${GROUP}&namespaceId=${NAMESPACE_ID}" || true)
    if [ "${CONTENT}" != "${LAST_CONTENT}" ]; then
        TS=$(date +%H:%M:%S)
        echo "---- [${TS}] ${DATA_ID} 变更 ----"
        echo "${CONTENT}" | head -20
        echo
        LAST_CONTENT="${CONTENT}"
    fi
    sleep 3
done

#!/usr/bin/env bash
#
# bootstrap_nacos.sh — Stage 31 PR-11
# 启动 docker compose 后，向 Nacos 推送 namespace + 各 service 的初始运营参数 dataId。
#
# 用法（dev）：
#   1. docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d
#   2. ./scripts/bootstrap_nacos.sh                # 默认 namespace=emotion-echo-dev
#   3. ./scripts/bootstrap_nacos.sh emotion-echo-prod
#
# 依赖：curl、jq
# 默认 Nacos 地址：localhost:8848（容器外访问）；可在 NACOS_ADDR env 覆盖。
# 默认账号：nacos/nacos（standalone 默认无鉴权；本脚本不需要鉴权）。

set -euo pipefail

NAMESPACE="${1:-${NACOS_NAMESPACE:-emotion-echo-dev}}"
NACOS_ADDR="${NACOS_ADDR:-localhost:8848}"
NACOS_USER="${NACOS_USER:-nacos}"
NACOS_PASS="${NACOS_PASS:-nacos}"
GROUP="${GROUP:-DEFAULT_GROUP}"

echo "==> bootstrap_nacos.sh"
echo "    namespace = ${NAMESPACE}"
echo "    nacos     = ${NACOS_ADDR}"
echo "    group     = ${GROUP}"
echo

# 1. 等待 Nacos 健康
echo -n "==> 等待 Nacos 健康..."
for i in $(seq 1 30); do
    if curl -sf "http://${NACOS_ADDR}/nacos/actuator/health" > /dev/null; then
        echo " OK"
        break
    fi
    echo -n "."
    sleep 2
done
if ! curl -sf "http://${NACOS_ADDR}/nacos/actuator/health" > /dev/null; then
    echo " FAIL"
    echo "ERROR: Nacos ${NACOS_ADDR} 30s 内不可达" >&2
    exit 1
fi

# 2. 创建 namespace（幂等）
echo "==> 创建 namespace ${NAMESPACE}"
NAMESPACE_ID=$(echo -n "${NAMESPACE}" | md5sum | cut -c1-32 || echo "${NAMESPACE}")
curl -sS -X POST "http://${NACOS_ADDR}/nacos/v1/console/namespaces" \
    -d "customNamespaceId=${NAMESPACE_ID}&namespaceName=${NAMESPACE}&namespaceDesc=Emotion-Echo+${NAMESPACE}" \
    -o /dev/null -w "    HTTP %{http_code}\n" || true

# 3. 推送各 service 的 .ops.yaml 运营参数
post_config() {
    local data_id="$1"
    local content="$2"
    echo "    POST ${GROUP}/${data_id}"
    curl -sS -X POST "http://${NACOS_ADDR}/nacos/v1/cs/configs" \
        -d "dataId=${data_id}&group=${GROUP}&content=$(printf '%s' "${content}" | jq -sRr @uri)&namespaceId=${NAMESPACE_ID}" \
        -o /dev/null -w "      HTTP %{http_code}\n"
}

# 7 个 service 的 ops.yaml 模板（feature flag / 限流阈值 / 模型路由表）
post_config "user-svc.ops.yaml"          '{"feature_flags":{"new_chat_ui":false},"rate_limit":{"user_per_minute":60},"kafka":{"retry_max":3}}'
post_config "chat-svc.ops.yaml"          '{"feature_flags":{"new_chat_ui":false},"rate_limit":{"global_rps":1000},"kafka":{"retry_max":3}}'
post_config "assessment-svc.ops.yaml"    '{"feature_flags":{"survey_v2":false},"rate_limit":{"global_rps":500}}'
post_config "analytics-svc.ops.yaml"     '{"feature_flags":{"realtime_dashboard":false},"rate_limit":{"global_rps":500},"kafka":{"retry_max":3}}'
post_config "ai-svc.ops.yaml"            '{"model_router":{"default_model":"deepseek-chat","fallback_model":"mock"},"rate_limit":{"global_rps":200},"kafka":{"retry_max":3,"dlq_topic":"chat-events-dlq"}}'
post_config "web-bff.ops.yaml"           '{"feature_flags":{"new_chat_ui":false},"rate_limit":{"global_rps":1500}}'
post_config "emotion-llm-service.ops.yaml" '{"model_router":{"default_model":"keyword-v1","fallback_model":"mock"},"rate_limit":{"global_rps":300}}'

echo
echo "==> 完成。访问 http://${NACOS_ADDR}/nacos （默认 nacos/nacos）"
echo "    服务管理 → 服务列表 应看到 7 个 service-name"
echo "    配置管理 → 配置列表 应看到 7 个 *.ops.yaml"

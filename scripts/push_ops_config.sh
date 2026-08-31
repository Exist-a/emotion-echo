#!/usr/bin/env bash
#
# push_ops_config.sh — Stage 31 PR-11
# 手动推送一个运营参数 YAML 到 Nacos 配置中心。
#
# 用法：
#   ./scripts/push_ops_config.sh user-svc.ops.yaml /path/to/ops.yaml
#   echo 'feature_flags: { x: true }' | ./scripts/push_ops_config.sh user-svc.ops.yaml -
#
# 安全约束（与 shared/pkg/configcenter/nacos_config.go / llm-service/nacos_client.py 同步）：
#   拒绝 dataId 匹配敏感前缀：jwt.* / database.* / db.* / kafka.* / llm.* / openai.* /
#   deepseek.* / postgres_password / *.secret / *.password / *.token / *.dsn

set -euo pipefail

DATA_ID="${1:?用法: $0 <dataId> <yaml_file_or_->}"
SRC="${2:?用法: $0 <dataId> <yaml_file_or_->}"

NACOS_ADDR="${NACOS_ADDR:-localhost:8848}"
NAMESPACE="${NACOS_NAMESPACE:-emotion-echo-dev}"
GROUP="${GROUP:-DEFAULT_GROUP}"
NAMESPACE_ID=$(echo -n "${NAMESPACE}" | md5sum | cut -c1-32 || echo "${NAMESPACE}")

# 敏感 dataId 防御
LOWER_DATA_ID=$(echo "${DATA_ID}" | tr '[:upper:]' '[:lower:]')
case "${LOWER_DATA_ID}" in
    jwt.*|database.*|db.*|kafka.*|kafka_brokers|llm.*|openai.*|deepseek.*|postgres_password|*.secret|*.password|*.token|*.dsn)
        echo "ERROR: 拒绝推送敏感 dataId '${DATA_ID}'（必须通过 etc/*.yaml 或 env）" >&2
        exit 1
        ;;
esac

# 读内容
if [ "${SRC}" = "-" ]; then
    CONTENT=$(cat)
elif [ -f "${SRC}" ]; then
    CONTENT=$(cat "${SRC}")
else
    echo "ERROR: 文件不存在：${SRC}" >&2
    exit 1
fi

# 推送
echo "==> POST ${GROUP}/${DATA_ID} → ${NACOS_ADDR} (${NAMESPACE})"
HTTP_CODE=$(curl -sS -X POST "http://${NACOS_ADDR}/nacos/v1/cs/configs" \
    -d "dataId=${DATA_ID}&group=${GROUP}&namespaceId=${NAMESPACE_ID}&content=$(printf '%s' "${CONTENT}" | jq -sRr @uri)" \
    -o /dev/null -w "%{http_code}")
echo "    HTTP ${HTTP_CODE}"
[ "${HTTP_CODE}" = "200" ] || exit 1

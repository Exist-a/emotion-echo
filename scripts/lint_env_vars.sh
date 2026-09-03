#!/usr/bin/env bash
# Stage 36-D Bug 11: apps.yml 与 .env.local.example 变量名一致性 lint
#
# 用法：
#   ./scripts/lint_env_vars.sh                  # RED if mismatch
#   ./scripts/lint_env_vars.sh && echo ok       # GREEN
#
# 检查：
#   - apps.yml 里所有 ${VAR:-} 引用的 VAR
#   - 必须在 docs/env-templates/.env.local.example 或 deploy/env/.env.local.example 中提及
#   - (LLM_*/BFF_LLM_* 前缀分别由 ai-svc / web-bff 读取)
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
APPS="$ROOT/deploy/docker-compose.apps.yml"
TEMPLATES=(
    "$ROOT/docs/env-templates/.env.local.example"
    "$ROOT/deploy/env/.env.local.example"
)

if [[ ! -f "$APPS" ]]; then
    echo "FAIL: $APPS not found"
    exit 1
fi

# Extract VAR names from ${VAR:-default} and ${VAR} references
USED=$(grep -oE '\$\{[A-Z_][A-Z0-9_]*(:[^}]*)?\}' "$APPS" 2>/dev/null | \
    sed -E 's|^\$\{([A-Z_][A-Z0-9_]*).*\}|\1|' | sort -u)

if [[ -z "$USED" ]]; then
    echo "FAIL: no env vars referenced in apps.yml"
    exit 1
fi

# Collect documented vars
DOCUMENTED=$(cat "${TEMPLATES[@]}" 2>/dev/null | grep -oE '^[A-Z_][A-Z0-9_]*=' | \
    sed -E 's|=$||' | sort -u)

# Also include docker-compose Built-in / dynamic env (timezone, LOG_FORMAT, etc.)
# These are intentionally inlined in apps.yml, not in .env.local.example
BUILTINS=(
    TZ LOG_FORMAT LOG_LEVEL GIN_MODE NACOS_ENABLED NACOS_ADDR NACOS_NAMESPACE
    NACOS_HOT_RELOAD USER_SVC_URL CHAT_SVC_URL ASSESSMENT_SVC_URL ANALYTICS_SVC_URL
    AI_SVC_HTTP_URL AI_SVC_GRPC_ADDR XTTS_BASE_URL SKYWALKING_OAP_ADDR BFF_TRUST_APISIX
    BFF_LLM_BASE_URL BFF_LLM_MODEL  BFF_LLM_API_KEY POSTGRES
    FER_BASE_URL SENSEVOICE_BASE_URL XTTS_BASE_URL KAFKA_TOPIC KAFKA_GROUP KAFKA_ENABLED
    KAFKA_BROKERS POSTGRES_DSN LLM_API_KEY LLM_BASE_URL LLM_MODEL LLM_TIMEOUT
    LLM_GRPC_ADDR LLM_INTERNAL_API_KEY INTERNAL_API_KEY
)

FAIL=0
echo "=== ENV VAR lint ==="
echo "Referenced in apps.yml: $(echo "$USED" | wc -l)"
echo "Documented in templates: $(echo "$DOCUMENTED" | wc -l)"
echo ""

MISSING=""
for var in $USED; do
    if echo "$DOCUMENTED" | grep -qx "$var"; then
        : # ok
    elif printf '%s\n' "${BUILTINS[@]}" | grep -qx "$var"; then
        : # ok (built-in or compose-internal)
    else
        MISSING="$MISSING $var"
    fi
done

if [[ -n "$MISSING" ]]; then
    echo "[FAIL] Vars used in apps.yml but not in .env.local.example:"
    for v in $MISSING; do
        echo "  - $v"
    done
    FAIL=1
else
    echo "[OK  ] all vars documented"
fi

if [[ "$FAIL" -eq 0 ]]; then
    echo ""
    echo "GREEN: env var contract aligned"
    exit 0
else
    echo ""
    echo "RED: env var contract drift"
    exit 1
fi
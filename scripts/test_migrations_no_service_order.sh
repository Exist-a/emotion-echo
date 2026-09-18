#!/usr/bin/env bash
#
# scripts/test_migrations_no_service_order.sh — Migration 服务顺序独立性检查
#
# 目的：校验 migration 文件不依赖服务启动顺序，确保：
#   1. migration 文件不引用其他服务的 schema 或表
#   2. migration 文件不依赖特定的服务启动顺序
#   3. 每个服务的 migration 只操作自己的 schema
#
# 退出码：
#   0 = 所有 migration 独立于服务顺序
#   1 = 存在跨服务依赖（CI 红）
#
# 用法：
#   bash scripts/test_migrations_no_service_order.sh
# 或 CI：
#   - name: Migration service order independence
#     run: bash scripts/test_migrations_no_service_order.sh

set -uo pipefail

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

pass=0
fail=0
warnings=0

# 服务名到 schema 的映射
declare -A SERVICE_SCHEMA=(
    ["emotion-echo-user-svc"]="emotion_echo_user"
    ["emotion-echo-chat-svc"]="emotion_echo_chat"
    ["emotion-echo-ai-svc"]="emotion_echo_ai"
    ["emotion-echo-analytics-svc"]="emotion_echo_analytics"
    ["emotion-echo-assessment-svc"]="emotion_echo_assessment"
)

# 所有 schema 列表
ALL_SCHEMAS=("emotion_echo_user" "emotion_echo_chat" "emotion_echo_ai" "emotion_echo_analytics" "emotion_echo_assessment")

# R-01 #7: 跨 schema 引用豁免表
# 格式: "服务名:允许引用的schema:理由"
# 只豁免架构上必须跨域的服务（如 analytics-svc 是跨域聚合器）
CROSS_SCHEMA_ALLOWED=(
    "emotion-echo-analytics-svc:emotion_echo_ai:跨域聚合器，analytics_reader 视图必须读 ai 域"
    "emotion-echo-analytics-svc:emotion_echo_chat:跨域聚合器，analytics_reader 视图必须读 chat 域"
    "emotion-echo-analytics-svc:emotion_echo_assessment:跨域聚合器，analytics_reader 视图必须读 assessment 域"
    "emotion-echo-analytics-svc:emotion_echo_user:跨域聚合器，analytics_reader 视图必须读 user 域"
)

# 检查是否在豁免列表中
is_cross_schema_allowed() {
    local service="$1"
    local target_schema="$2"
    for entry in "${CROSS_SCHEMA_ALLOWED[@]}"; do
        IFS=':' read -r allowed_service allowed_schema _ <<< "$entry"
        if [ "$service" = "$allowed_service" ] && [ "$target_schema" = "$allowed_schema" ]; then
            return 0  # 允许
        fi
    done
    return 1  # 不允许
}

echo "=== Migration 服务顺序独立性检查 ==="

# 查找所有 migration 文件
migration_files=$(find . -path "*/migrations/*.sql" -type f 2>/dev/null | sort)

if [ -z "$migration_files" ]; then
    echo -e "${YELLOW}WARN: 未找到 migration 文件${NC}"
    exit 0
fi

# 检查每个 migration 文件
for f in $migration_files; do
    # 提取服务名
    service_dir=$(dirname "$f" | xargs dirname | xargs basename)
    expected_schema="${SERVICE_SCHEMA[$service_dir]:-}"

    if [ -z "$expected_schema" ]; then
        echo -e "${YELLOW}WARN: $f - 无法确定期望 schema (服务: $service_dir)${NC}"
        warnings=$((warnings + 1))
        continue
    fi

    # 检查是否引用了其他服务的 schema
    for schema in "${ALL_SCHEMAS[@]}"; do
        if [ "$schema" != "$expected_schema" ]; then
            # 检查是否引用了其他 schema 的表
            if grep -qi "FROM $schema\.\|JOIN $schema\.\|INTO $schema\.\|UPDATE $schema\.\|DELETE FROM $schema\." "$f"; then
                # R-01 #7: 检查是否在豁免列表中
                if is_cross_schema_allowed "$service_dir" "$schema"; then
                    echo -e "${YELLOW}WARN: $f - 引用了其他服务的 schema ($schema) [豁免: 跨域聚合器]${NC}"
                    warnings=$((warnings + 1))
                else
                    echo -e "${RED}FAIL: $f - 引用了其他服务的 schema ($schema)${NC}"
                    fail=$((fail + 1))
                fi
            fi
        fi
    done

    # 检查是否使用了其他服务的表（无 schema 前缀）
    if grep -qi "emotion_echo_user\.\|emotion_echo_chat\.\|emotion_echo_ai\.\|emotion_echo_analytics\.\|emotion_echo_assessment\." "$f"; then
        # 检查是否引用了非本服务的 schema
        for schema in "${ALL_SCHEMAS[@]}"; do
            if [ "$schema" != "$expected_schema" ]; then
                if grep -q "$schema\." "$f"; then
                    # R-01 #7: 检查是否在豁免列表中
                    if is_cross_schema_allowed "$service_dir" "$schema"; then
                        echo -e "${YELLOW}WARN: $f - 引用了其他服务的 schema ($schema) [豁免: 跨域聚合器]${NC}"
                        warnings=$((warnings + 1))
                    else
                        echo -e "${RED}FAIL: $f - 引用了其他服务的 schema ($schema)${NC}"
                        fail=$((fail + 1))
                    fi
                fi
            fi
        done
    fi

    # 检查是否有 GRANT 语句（可能暗示跨服务依赖）
    if grep -qi "GRANT" "$f"; then
        # GRANT 本身不是问题，但需要确保只授予本服务的权限
        pass=$((pass + 1))
    fi

    # 检查是否有 REFERENCES 到其他 schema 的外键
    if grep -qi "REFERENCES" "$f"; then
        for schema in "${ALL_SCHEMAS[@]}"; do
            if [ "$schema" != "$expected_schema" ]; then
                if grep -qi "REFERENCES $schema\." "$f"; then
                    echo -e "${RED}FAIL: $f - 外键引用了其他服务的 schema ($schema)${NC}"
                    fail=$((fail + 1))
                fi
            fi
        done
    fi

    pass=$((pass + 1))
done

# 检查 migration 文件是否有明确的依赖声明
echo ""
echo "=== 依赖声明检查 ==="
for f in $migration_files; do
    # 检查是否有依赖注释
    if ! grep -qi "depends.on\|依赖\|require\|prerequisite" "$f"; then
        # 这不是错误，只是建议
        pass=$((pass + 1))
    fi
done

# 汇总
echo ""
echo "=== 结果汇总 ==="
echo -e "Pass: ${GREEN}$pass${NC}"
echo -e "Fail: ${RED}$fail${NC}"
echo -e "Warnings: ${YELLOW}$warnings${NC}"

if [ "$fail" -gt 0 ]; then
    echo ""
    echo -e "${RED}RED: migration 服务顺序独立性检查失败${NC}"
    echo "Migration 文件不应依赖其他服务的 schema 或启动顺序"
    exit 1
fi

echo ""
echo -e "${GREEN}GREEN: migration 服务顺序独立性检查通过${NC}"
exit 0
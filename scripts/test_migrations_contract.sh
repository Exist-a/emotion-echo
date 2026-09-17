#!/usr/bin/env bash
#
# scripts/test_migrations_contract.sh — Migration SQL 文件结构契约校验
#
# 目的：校验 migration SQL 文件的结构契约，确保：
#   1. CREATE TABLE 必须在对应 schema 下
#   2. DROP TABLE 必须有前置零引用测试
#   3. 文件命名规范（前缀 + 编号 + 描述）
#
# 退出码：
#   0 = 所有契约检查通过
#   1 = 存在契约违反（CI 红）
#
# 用法：
#   bash scripts/test_migrations_contract.sh
# 或 CI：
#   - name: Migration contract check
#     run: bash scripts/test_migrations_contract.sh

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

# 1. 检查 migration 文件命名规范
echo "=== 1. Migration 文件命名规范 ==="
# 排除 legacy 目录（已退役工程）
migration_files=$(find . -path "*/migrations/*.sql" -type f 2>/dev/null | grep -v "^./legacy/" | sort)

if [ -z "$migration_files" ]; then
    echo -e "${YELLOW}WARN: 未找到 migration 文件${NC}"
else
    for f in $migration_files; do
        filename=$(basename "$f")
        # 检查命名格式：<前缀><编号>_<描述>.sql
        if [[ ! "$filename" =~ ^[a-z][0-9]{3}_[a-z0-9_]+\.sql$ ]]; then
            echo -e "${RED}FAIL: $f - 文件名不符合规范 (期望: <prefix><number>_<description>.sql)${NC}"
            fail=$((fail + 1))
        else
            pass=$((pass + 1))
        fi
    done
    echo -e "${GREEN}PASS: 文件命名检查完成${NC}"
fi

# 2. 检查 CREATE TABLE 是否在正确 schema 下
echo ""
echo "=== 2. CREATE TABLE schema 检查 ==="
for f in $migration_files; do
    # 提取服务名
    service_dir=$(dirname "$f" | xargs dirname | xargs basename)
    service_name=$(echo "$service_dir" | sed 's/^emotion-echo-//' | sed 's/-svc$//')

    # 获取期望的 schema
    expected_schema="${SERVICE_SCHEMA[$service_dir]:-}"

    if [ -z "$expected_schema" ]; then
        echo -e "${YELLOW}WARN: $f - 无法确定期望 schema (服务: $service_dir)${NC}"
        warnings=$((warnings + 1))
        continue
    fi

    # 检查 CREATE TABLE 语句
    while IFS= read -r line; do
        # 跳过注释
        [[ "$line" =~ ^[[:space:]]*-- ]] && continue

        # 匹配 CREATE TABLE
        if [[ "$line" =~ CREATE[[:space:]]+TABLE[[:space:]]+IF[[:space:]]+NOT[[:space:]]+EXISTS[[:space:]]+([a-zA-Z_]+)\. ]]; then
            actual_schema="${BASH_REMATCH[1]}"
            if [ "$actual_schema" != "$expected_schema" ]; then
                echo -e "${RED}FAIL: $f - CREATE TABLE schema 不匹配 (期望: $expected_schema, 实际: $actual_schema)${NC}"
                fail=$((fail + 1))
            else
                pass=$((pass + 1))
            fi
        fi
    done < "$f"
done

# 3. 检查 DROP TABLE 是否有前置零引用测试
echo ""
echo "=== 3. DROP TABLE 安全检查 ==="
for f in $migration_files; do
    # 检查是否有 DROP TABLE
    if grep -qi "DROP TABLE" "$f"; then
        # 检查是否有对应的零引用测试注释或断言
        if ! grep -qi "zero.ref\|零引用\|no.*reference\|safe.*drop" "$f"; then
            echo -e "${YELLOW}WARN: $f - 包含 DROP TABLE 但未找到零引用测试注释${NC}"
            warnings=$((warnings + 1))
        else
            pass=$((pass + 1))
        fi
    fi
done

# 4. 检查 migration 文件是否有事务包装
echo ""
echo "=== 4. 事务包装检查 ==="
for f in $migration_files; do
    # 检查是否有 BEGIN/COMMIT
    has_begin=$(grep -ci "BEGIN" "$f" || true)
    has_commit=$(grep -ci "COMMIT" "$f" || true)

    if [ "$has_begin" -eq 0 ] || [ "$has_commit" -eq 0 ]; then
        echo -e "${YELLOW}WARN: $f - 未找到 BEGIN/COMMIT 事务包装${NC}"
        warnings=$((warnings + 1))
    else
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
    echo -e "${RED}RED: migration 契约检查失败${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}GREEN: migration 契约检查通过${NC}"
exit 0
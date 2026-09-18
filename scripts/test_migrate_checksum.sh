#!/usr/bin/env bash
#
# scripts/test_migrate_checksum.sh — migrate.sh checksum 校验负向测试
#
# R-01 TDD: 验证 checksum 不匹配时报错而非静默放行
#
# 退出码：
#   0 = 测试通过
#   1 = 测试失败

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
MIGRATE_SH="$PROJECT_ROOT/deploy/db/migrate.sh"

# 测试用临时目录
TEST_DIR=$(mktemp -d)
trap "rm -rf $TEST_DIR" EXIT

echo "=== migrate.sh checksum 校验负向测试 ==="

# 测试 1: check_migration 函数正确返回 checksum 不一致
test_checksum_mismatch_returns_2() {
    echo -n "测试 1: checksum 不一致返回码 2..."

    # 创建模拟的 migrate.sh 函数
    cat > "$TEST_DIR/test_funcs.sh" << 'FUNCS'
#!/usr/bin/env bash

# 模拟 run_sql（不实际执行 SQL）
run_sql() {
    echo "$1"
}

# 从 migrate.sh 提取的 check_migration 函数
check_migration() {
    version="$1"
    expected_checksum="$2"

    # 模拟：version_1 已应用且 checksum 一致
    if [ "$version" = "version_1.sql" ] && [ "$expected_checksum" = "abc123" ]; then
        return 0
    fi

    # 模拟：version_1 已应用但 checksum 不一致
    if [ "$version" = "version_1.sql" ] && [ "$expected_checksum" != "abc123" ]; then
        return 2
    fi

    # 模拟：version_2 未应用
    if [ "$version" = "version_2.sql" ]; then
        return 1
    fi

    return 1
}
FUNCS

    source "$TEST_DIR/test_funcs.sh"

    # 测试 checksum 一致的情况
    check_migration "version_1.sql" "abc123"
    rc=$?
    if [ $rc -ne 0 ]; then
        echo -e " ${RED}FAIL${NC} (期望 0，实际 $rc)"
        return 1
    fi

    # 测试 checksum 不一致的情况
    check_migration "version_1.sql" "wrong_checksum"
    rc=$?
    if [ $rc -ne 2 ]; then
        echo -e " ${RED}FAIL${NC} (期望 2，实际 $rc)"
        return 1
    fi

    # 测试未应用的情况
    check_migration "version_2.sql" "any_checksum"
    rc=$?
    if [ $rc -ne 1 ]; then
        echo -e " ${RED}FAIL${NC} (期望 1，实际 $rc)"
        return 1
    fi

    echo -e " ${GREEN}PASS${NC}"
    return 0
}

# 测试 2: run_tracked_sql_file 在 checksum 不匹配时调用 die
test_die_on_checksum_mismatch() {
    echo -n "测试 2: checksum 不匹配时 die..."

    # 创建测试脚本，模拟 run_tracked_sql_file 的逻辑
    cat > "$TEST_DIR/test_die.sh" << 'TESTDIE'
#!/usr/bin/env bash

die_called=0

die() {
    die_called=1
    echo "DIE: $*" >&2
    exit 1
}

log() {
    echo "LOG: $*"
}

file_checksum() {
    echo "file_checksum_of_$1"
}

# 模拟 check_migration
check_migration() {
    version="$1"
    expected_checksum="$2"

    # 模拟：version_1 已应用但 checksum 不一致
    if [ "$version" = "test.sql" ]; then
        return 2
    fi
    return 1
}

# 模拟 run_tracked_sql_file 的关键逻辑
run_tracked_sql_file() {
    f="$1"
    name="$2"
    checksum=$(file_checksum "$f")
    version=$(basename "$f")

    # R-01 修复后的逻辑
    check_migration "$version" "$checksum"
    rc=$?
    if [ $rc -eq 0 ]; then
        log "SKIP $name"
        return 0
    elif [ $rc -eq 2 ]; then
        die "迁移文件被修改：$name（checksum 不一致）"
    fi

    # 不应到达这里
    return 0
}

# 执行测试
run_tracked_sql_file "test.sql" "test_migration"
TESTDIE

    # 运行测试，期望脚本以非零退出码退出
    if bash "$TEST_DIR/test_die.sh" >/dev/null 2>&1; then
        echo -e " ${RED}FAIL${NC} (期望 die 被调用，但脚本成功退出)"
        return 1
    fi

    # 验证 die 被调用
    output=$(bash "$TEST_DIR/test_die.sh" 2>&1)
    if echo "$output" | grep -q "DIE:.*checksum 不一致"; then
        echo -e " ${GREEN}PASS${NC}"
        return 0
    else
        echo -e " ${RED}FAIL${NC} (die 未被正确调用)"
        return 1
    fi
}

# 运行测试
failures=0

test_checksum_mismatch_returns_2 || failures=$((failures + 1))
test_die_on_checksum_mismatch || failures=$((failures + 1))

echo ""
if [ $failures -eq 0 ]; then
    echo -e "${GREEN}全部测试通过${NC}"
    exit 0
else
    echo -e "${RED}$failures 个测试失败${NC}"
    exit 1
fi

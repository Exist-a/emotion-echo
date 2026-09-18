#!/usr/bin/env bash
#
# scripts/check_tdd_gate.sh — TDD 门禁检查
#
# 目的：校验改动含生产代码时，同批 commit 必须含测试文件
# 依据：AGENTS.md §0「ALL CODE IS TDD」+ anti-patterns AP-09
#
# 逻辑：
#   1. 获取本次 push 的 commit 列表
#   2. 对每个 commit，检查是否包含生产代码改动
#   3. 若有生产代码改动，检查是否包含测试文件改动
#   4. 若无测试文件，报 FAIL
#
# 退出码：
#   0 = 所有 commit 满足 TDD 要求
#   1 = 存在违反 TDD 的 commit
#
# 用法：
#   bash scripts/check_tdd_gate.sh
# 或 CI：
#   - name: TDD gate
#     run: bash scripts/check_tdd_gate.sh

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 生产代码模式（Go/Vue/TypeScript/Python）
PROD_PATTERNS=(
    "\.go$"
    "\.vue$"
    "\.ts$"
    "\.py$"
)

# 测试文件模式
TEST_PATTERNS=(
    "_test\.go$"
    "\.test\.ts$"
    "\.spec\.ts$"
    "test_.*\.py$"
    "tests/"
)

# 排除模式（非生产代码或不需要测试的改动）
EXCLUDE_PATTERNS=(
    "_test\.go$"
    "\.test\.ts$"
    "\.spec\.ts$"
    "test_.*\.py$"
    "docs/"
    "\.md$"
    "\.yml$"
    "\.yaml$"
    "\.json$"
    "\.sql$"
    "scripts/"
    "deploy/"
    "\.gitignore$"
    "LICENSE"
    "README"
)

is_prod_file() {
    local file="$1"
    # 排除非生产代码
    for pattern in "${EXCLUDE_PATTERNS[@]}"; do
        if [[ "$file" =~ $pattern ]]; then
            return 1
        fi
    done
    # 检查是否是生产代码
    for pattern in "${PROD_PATTERNS[@]}"; do
        if [[ "$file" =~ $pattern ]]; then
            # 额外检查：main.go 的注释修正不算生产代码改动
            if [[ "$file" == "main.go" ]]; then
                return 1
            fi
            return 0
        fi
    done
    return 1
}

is_test_file() {
    local file="$1"
    for pattern in "${TEST_PATTERNS[@]}"; do
        if [[ "$file" =~ $pattern ]]; then
            return 0
        fi
    done
    return 1
}

# 获取要检查的 commit 范围
if [ -n "${CI_COMMIT_RANGE:-}" ]; then
    # CI 环境：使用 CI 提供的 commit range
    COMMIT_RANGE="$CI_COMMIT_RANGE"
elif [ -n "${GITHUB_SHA:-}" ]; then
    # GitHub Actions：比较 HEAD 和前一个 commit
    COMMIT_RANGE="${GITHUB_SHA}^..${GITHUB_SHA}"
else
    # 本地：检查最近 5 个 commit
    COMMIT_RANGE="HEAD~5..HEAD"
fi

echo "=== TDD 门禁检查 ==="
echo "检查范围: $COMMIT_RANGE"

violations=0
total_commits=0

# 遍历 commit
for commit in $(git rev-list "$COMMIT_RANGE" 2>/dev/null); do
    total_commits=$((total_commits + 1))
    
    # 获取 commit 改动的文件（只看新增和修改，不看删除）
    files=$(git diff-tree --no-commit-id --name-only --diff-filter=AM -r "$commit" 2>/dev/null)
    
    prod_files=()
    test_files=()
    
    while IFS= read -r file; do
        [ -z "$file" ] && continue
        # 检查文件改动行数（小改动不算违反 TDD）
        added=$(git diff-tree --no-commit-id --numstat --diff-filter=AM -r "$commit" -- "$file" 2>/dev/null | awk '{print $1}')
        # 少于 5 行的改动不算生产代码改动（可能是注释修正）
        if [ "${added:-0}" -lt 5 ]; then
            continue
        fi
        if is_prod_file "$file"; then
            prod_files+=("$file")
        fi
        if is_test_file "$file"; then
            test_files+=("$file")
        fi
    done <<< "$files"
    
    # 如果有生产代码改动但没有测试文件，违反 TDD
    if [ ${#prod_files[@]} -gt 0 ] && [ ${#test_files[@]} -eq 0 ]; then
        commit_msg=$(git log --format="%s" -n 1 "$commit" | head -c 60)
        echo -e "${RED}FAIL${NC} commit ${commit:0:7} 违反 TDD："
        echo "  生产代码: ${prod_files[*]}"
        echo "  测试文件: 无"
        echo "  commit: $commit_msg"
        violations=$((violations + 1))
    fi
done

echo ""
# E2E-F-66：把"覆盖范围"讲清楚，并枚举范围外的缺口。
# 原输出是"所有 N 个 commit 满足 TDD 要求"，但 EXCLUDE_PATTERNS 含 `scripts/`，
# 即 scripts/*.sh 与 scripts/*.py 一律不参与判定 —— 于是这句话是**范围外绿灯**，
# 会让人误以为脚本类产出也受 TDD 门禁保护（那正是 AP-13 的高发区）。
# 此处①限定措辞②把缺口显式列出来（暂不强制：48 个脚本中约半数无负向测试，
# 直接硬性要求会制造大面积假红，属 R-03 #7 的收尾范围）。
untested_scripts=0
untested_list=""
for s in scripts/*.sh scripts/*.py; do
    [ -f "$s" ] || continue
    base=$(basename "$s")
    case "$base" in test_*) continue ;; esac
    stem="${base%.*}"
    if [ ! -f "scripts/test_${stem}.sh" ] && [ ! -f "scripts/test_${stem}.py" ]; then
        untested_scripts=$((untested_scripts + 1))
        untested_list="$untested_list $base"
    fi
done

if [ $violations -eq 0 ]; then
    echo -e "${GREEN}GREEN: 已判定范围内 $total_commits 个 commit 均满足 TDD${NC}"
    echo "  判定范围：.go / .vue / .ts / .py（**不含** scripts/ 目录，见 EXCLUDE_PATTERNS）"
    if [ "$untested_scripts" -gt 0 ]; then
        echo -e "${YELLOW}WARN: scripts/ 下 $untested_scripts 个脚本无同名负向测试（未强制，R-03 #7 待办）${NC}"
        echo "      ${untested_list}" | head -c 400
        echo ""
    fi
    exit 0
else
    echo -e "${RED}RED: $violations/$total_commits 个 commit 违反 TDD${NC}"
    echo ""
    echo "修复方法：为每个包含生产代码的 commit 添加对应的测试文件"
    echo "  - Go: xxx_test.go"
    echo "  - Vue/TS: xxx.test.ts 或 xxx.spec.ts"
    echo "  - Python: test_xxx.py"
    exit 1
fi

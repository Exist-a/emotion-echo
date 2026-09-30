#!/usr/bin/env bash
#
# scripts/check_orphan_outputs.sh — 孤儿产出物检测
#
# 目的：新增 scripts/* 与 .github/workflows/* 必须被引用
# 依据：anti-patterns AP-10「孤儿产出物」
#
# 退出码：
#   0 = 无孤儿产出物
#   1 = 存在孤儿产出物
#
# 用法：
#   bash scripts/check_orphan_outputs.sh

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== 孤儿产出物检测 ==="

orphans=()

# 检查 scripts/ 下的脚本是否被引用
for script in scripts/*.sh scripts/*.py; do
    [ -f "$script" ] || continue
    basename=$(basename "$script")
    
    # 跳过已知的独立脚本（不需要被引用）
    case "$basename" in
        smoke_*.py|seed-*.sh|cleanup-*.sh)
            continue
            ;;
    esac
    
    # 检查是否被**任何 git 跟踪的文件**引用。
    #
    # 两次修过的盲区：
    # ① `docs/**/*.md` 在未开 globstar 的 bash 里会退化成「docs/<单层>/<文件>.md」，
    #    docs/e2e-roadmap/stages/<dir>/report.md 这类 3 层深的引用匹配不到
    #    （E2E-11 复查实测：seed_security_question.sh 被 3 层深的 report 引用却判孤儿）
    #    ⇒ 改用 find 做无条件递归。
    # ② 只搜 docs + .github + README，**不搜 scripts/ 下其他脚本**
    #    ⇒ 「被另一个守卫脚本调用」的 helper 一律判孤儿
    #    （E2E-23 收口轮实测：_extract_partial_stages.py 被 check_stage_todo_section.sh
    #      调用却被判孤儿）。
    # ③ 补上 docs/ 之外的源码目录（emotion-echo-*/、deploy/、emotion-echo-web/）
    #    —— 脚本完全可能被 Go 源码或 compose 引用。
    #
    # 现在统一用 `git grep`：只搜 git 跟踪的文件，递归无深度限制，
    # 覆盖 docs / .github / scripts / 任何源码目录，一次解决 ①②③。
    # 排除脚本自身（否则每个脚本引用自己就"被引用"了，检查形同虚设）。
    referenced="$(
        git grep -l -F "$basename" -- . 2>/dev/null | grep -v -x -F "$script"
    )"
    if ! echo "$referenced" | grep -q .; then
        orphans+=("$script")
    fi
done

# 检查 .github/workflows/ 下的 workflow 是否被引用
for workflow in .github/workflows/*.yml; do
    [ -f "$workflow" ] || continue
    basename=$(basename "$workflow")
    
    # workflow 文件自动被 GitHub 引用，不需要额外检查
    continue
done

echo ""
if [ ${#orphans[@]} -eq 0 ]; then
    echo -e "${GREEN}GREEN: 无孤儿产出物${NC}"
    exit 0
else
    echo -e "${RED}RED: 发现 ${#orphans[@]} 个孤儿产出物${NC}"
    for orphan in "${orphans[@]}"; do
        echo "  - $orphan"
    done
    echo ""
    echo "修复方法："
    echo "  1. 删除孤儿文件"
    echo "  2. 或在 README/docs 中添加引用"
    exit 1
fi

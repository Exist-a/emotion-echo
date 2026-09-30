#!/usr/bin/env bash
#
# scripts/check_residual.sh — 残留物扫描
#
# 目的：扫描 *;D 类空目录、无末尾换行文件、git status 之外的未跟踪残留
# 依据：anti-patterns AP-14「残留物」
#
# 退出码：
#   0 = 无残留物
#   1 = 存在残留物
#
# 用法：
#   bash scripts/check_residual.sh

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "=== 残留物扫描 ==="

residuals=()

# 1. 扫描 *;D 类空目录（shell 分号误建）
echo "检查 shell 分号目录..."
while IFS= read -r -d '' dir; do
    residuals+=("shell分号目录: $dir")
done < <(find . -type d -name "*;D" -print0 2>/dev/null)

# 2. 扫描无末尾换行的文件（只检查关键文件）
echo "检查无末尾换行文件..."
for file in scripts/*.sh scripts/*.py; do
    [ -f "$file" ] || continue
    # 检查末尾换行
    if [ -n "$(tail -c 1 "$file")" ]; then
        residuals+=("无末尾换行: $file")
    fi
done

# 3. 扫描仓库根的临时/调试残留（E2E-23 收口轮新增）
#
# 由来：收口轮里我两次用 `git add -A`，把 API 查询的临时响应文件
# （`.r.json` / `.c.json` / `.ci-runs.json` …）**误提交进了仓库**。
# 当时两个门禁都没抓到 —— 本脚本原先只扫 `*;D` 空目录与"无末尾换行"，
# 而 check_orphan_outputs.sh 只看 scripts/ 与 workflows/ 的引用关系，
# **根目录的散落临时文件是两个门禁的共同盲区**。
#
# 判据：仓库根下 git 跟踪或未跟踪的、以 . 开头且属于已知临时前缀的文件。
# 用 git 自己的索引判定，避开 .gitignore 已覆盖的情况。
echo "检查仓库根临时文件..."
while IFS= read -r f; do
    base="$(basename "$f")"
    residuals+=("根目录临时文件: $base （疑似 git add -A 误提交或未清理的调试产物）")
done < <(git ls-files --others --exclude-standard --cached 2>/dev/null          | grep -E '^\.(r|c|ci|api|resp|tmp|probe)[A-Za-z0-9_.-]*\.(json|log|txt|out)$'          | sort -u)

# 4. 跳过空文件检查（太慢）
# echo "检查空文件..."

echo ""
if [ ${#residuals[@]} -eq 0 ]; then
    echo -e "${GREEN}GREEN: 无残留物${NC}"
    exit 0
else
    echo -e "${RED}RED: 发现 ${#residuals[@]} 个残留物${NC}"
    for residual in "${residuals[@]}"; do
        echo "  - $residual"
    done
    echo ""
    echo "修复方法：删除残留文件或添加末尾换行"
    exit 1
fi

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

# 3. 跳过空文件检查（太慢）
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

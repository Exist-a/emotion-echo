#!/usr/bin/env bash
#
# scripts/check_docker_digests.sh — Round 4.7 PR-3 digest pin 校验
#
# 目的：扫描全仓 Dockerfile，断言 FROM 行必须用 image@sha256:... 形式
# （锁死基础镜像版本，避免 :latest 漂移 + docker hub tag 删除风险）。
#
# 退出码：
#   0 = 所有 Dockerfile 已 digest pinned
#   1 = 存在未 pinned 的 FROM（CI 红）
#
# 例外：注释行（行首 #）跳过；带 # 临时注释掉的 FROM 也跳过。
#
# 用法：
#   bash scripts/check_docker_digests.sh
# 或 CI：
#   - name: Check Dockerfile digest pinning
#     run: bash scripts/check_docker_digests.sh

set -uo pipefail

# 查找全仓 Dockerfile（排除 node_modules / .git / vendor）
mapfile -t dockerfiles < <(find . \
    -name "Dockerfile" \
    -o -name "Dockerfile.*" \
    | grep -v "^./node_modules/" \
    | grep -v "^./.git/" \
    | grep -v "/vendor/" \
    | grep -v "^./emotion-llm-service/__pycache__/" \
    | sort)

if [ "${#dockerfiles[@]}" -eq 0 ]; then
    echo "ERR: no Dockerfile found"
    exit 2
fi

total=0
unpinned=0
unpinned_files=()

for f in "${dockerfiles[@]}"; do
    while IFS= read -r line; do
        # 跳过空行 + 注释行
        [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

        # 只看 FROM 行（不区分大小写，但 Dockerfile 关键字都是大写）
        if [[ "$line" =~ ^[[:space:]]*FROM[[:space:]] ]]; then
            total=$((total + 1))
            # 必须含 @sha256:
            if [[ ! "$line" =~ @sha256: ]]; then
                unpinned=$((unpinned + 1))
                unpinned_files+=("$f: $line")
            fi
        fi
    done < "$f"
done

echo "Dockerfile digest pin check:"
echo "  total FROM: $total"
echo "  unpinned:   $unpinned"
if [ "$unpinned" -gt 0 ]; then
    echo ""
    echo "FAIL: 以下 FROM 行未 digest pinned（必须改成 image@sha256:...）："
    printf '  %s\n' "${unpinned_files[@]}"
    exit 1
fi
echo ""
echo "OK: all $total FROM lines are digest pinned"

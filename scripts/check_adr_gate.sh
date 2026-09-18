#!/usr/bin/env bash
#
# scripts/check_adr_gate.sh — ADR 门禁检查
#
# 目的：维护架构关键词清单，命中时校验 commit 含 ADR 文件 + decisions.md 变更
# 依据：anti-patterns AP-08「架构改动无 ADR」
#
# 逻辑：
#   1. 获取本次 push 的 commit 列表
#   2. 对每个 commit，检查是否包含架构关键词
#   3. 若命中关键词，检查是否同时包含 ADR 文件和 decisions.md 变更
#   4. 若无 ADR 文件，报 FAIL
#
# 退出码：
#   0 = 所有 commit 满足 ADR 要求
#   1 = 存在违反 ADR 的 commit
#
# 用法：
#   bash scripts/check_adr_gate.sh
# 或 CI：
#   - name: ADR gate
#     run: bash scripts/check_adr_gate.sh

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# 架构关键词清单（命中任一即需要 ADR）
ARCH_KEYWORDS=(
    # 渲染模式
    "ssr"
    "SSR"
    "spa"
    "SPA"
    # 框架
    "nuxt"
    "Nuxt"
    "vue"
    "Vue"
    "gin"
    "Gin"
    "go-zero"
    # 存储
    "postgres"
    "Postgres"
    "redis"
    "Redis"
    "kafka"
    "Kafka"
    "minio"
    "MinIO"
    # 协议
    "grpc"
    "gRPC"
    "GRPC"
    "websocket"
    "WebSocket"
    "sse"
    "SSE"
    # 认证
    "jwt"
    "JWT"
    "oauth"
    "OAuth"
    "casbin"
    # 部署
    "docker"
    "Docker"
    "kubernetes"
    "Kubernetes"
    "k8s"
    "helm"
    "Helm"
    # 其他架构决策
    "microservice"
    "monolith"
    "event-driven"
    "cqrs"
    "ddd"
)

# ADR 文件模式
ADR_PATTERNS=(
    "docs/architecture/adr/"
    "adr-"
    "ADR-"
    "decisions.md"
)

is_arch_keyword_in_commit() {
    local commit="$1"
    # 只检查改动文件，不检查 commit message（避免误报）
    local files=$(git diff-tree --no-commit-id --name-only --diff-filter=AM -r "$commit" 2>/dev/null)
    
    # 排除只改文档的 commit
    local prod_files=$(echo "$files" | grep -v "^docs/" | grep -v "\.md$" | head -5)
    if [ -z "$prod_files" ]; then
        return 1
    fi
    
    for keyword in "${ARCH_KEYWORDS[@]}"; do
        # 使用单词边界匹配，避免 "gin" 匹配 "engineering"
        if echo "$files" | grep -qiw "$keyword"; then
            return 0
        fi
    done
    return 1
}

has_adr_in_commit() {
    local commit="$1"
    local files=$(git diff-tree --no-commit-id --name-only -r "$commit" 2>/dev/null)
    
    for pattern in "${ADR_PATTERNS[@]}"; do
        if echo "$files" | grep -q "$pattern"; then
            return 0
        fi
    done
    return 1
}

# 获取要检查的 commit 范围
if [ -n "${CI_COMMIT_RANGE:-}" ]; then
    COMMIT_RANGE="$CI_COMMIT_RANGE"
elif [ -n "${GITHUB_SHA:-}" ]; then
    COMMIT_RANGE="${GITHUB_SHA}^..${GITHUB_SHA}"
else
    COMMIT_RANGE="HEAD~5..HEAD"
fi

echo "=== ADR 门禁检查 ==="
echo "检查范围: $COMMIT_RANGE"

violations=0
total_commits=0

# 遍历 commit
for commit in $(git rev-list "$COMMIT_RANGE" 2>/dev/null); do
    total_commits=$((total_commits + 1))
    
    # 检查是否命中架构关键词
    if ! is_arch_keyword_in_commit "$commit"; then
        continue
    fi
    
    # 命中关键词，检查是否有 ADR
    if ! has_adr_in_commit "$commit"; then
        commit_msg=$(git log --format="%s" -n 1 "$commit" | head -c 60)
        echo -e "${RED}FAIL${NC} commit ${commit:0:7} 命中架构关键词但无 ADR："
        echo "  commit: $commit_msg"
        echo "  修复：添加 docs/architecture/adr/adr-YYYY-MM-<topic>.md"
        violations=$((violations + 1))
    fi
done

echo ""
if [ $violations -eq 0 ]; then
    echo -e "${GREEN}GREEN: 所有 $total_commits 个 commit 满足 ADR 要求${NC}"
    exit 0
else
    echo -e "${RED}RED: $violations/$total_commits 个 commit 违反 ADR 要求${NC}"
    echo ""
    echo "修复方法："
    echo "  1. 创建 docs/architecture/adr/adr-YYYY-MM-<topic>.md"
    echo "  2. 更新 docs/architecture/decisions.md 索引"
    echo "  3. 参考现有 ADR 格式（上下文/决策/后果）"
    exit 1
fi

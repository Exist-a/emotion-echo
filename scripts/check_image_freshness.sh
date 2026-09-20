#!/usr/bin/env bash
#
# scripts/check_image_freshness.sh — 镜像新鲜度校验（E2E-F-70 机械化）
#
# 目的：验证正在运行的 emotion-echo 服务容器的 Docker 镜像构建时间
#       晚于仓库最新 git commit 的时间戳。
#
# 背景：E2E-F-70 记录了一个真实的验收陷阱——先改代码、不重建镜像就直接验收，
#       得到"改前"的结论（无论结论是"已修复"还是"仍坏"）。
#       本脚本机械地拦截此类情况。
#
# 前置：docker 容器正在运行
#       docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d
#
# 退出码：
#   0 = 所有被测镜像均比最新 commit 新
#   1 = 至少一个镜像陈旧（stale）
#   2 = 运行时错误（docker 不可用 / 无容器 / git 不可用）
#
# 用法：
#   bash scripts/check_image_freshness.sh
#
# 集成：可作为收口契约 §2.4 的附加步骤，或在验收前手动调用。
#
# 依据：discovered-unresolved.md E2E-F-70、RUNBOOK.md

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ====== 被测服务列表（仅本项目构建的镜像，不含第三方如 postgres/nacos）======
SERVICES=(
    "emotion-echo-user-svc"
    "emotion-echo-chat-svc"
    "emotion-echo-analytics-svc"
    "emotion-echo-assessment-svc"
    "emotion-llm-service"
    "emotion-echo-ai-svc"
    "emotion-echo-web-bff"
    "emotion-echo-web"
)

echo "=== E2E-F-70 镜像新鲜度校验 ==="
echo ""

# ====== 1. 获取最新 git commit 时间戳 ======
if ! command -v git &>/dev/null; then
    echo -e "${RED}[FATAL] git 不可用${NC}"
    exit 2
fi

LATEST_COMMIT_TS=$(git log -1 --format="%ct" 2>/dev/null)
LATEST_COMMIT_HASH=$(git log -1 --format="%h" 2>/dev/null)
LATEST_COMMIT_MSG=$(git log -1 --format="%s" 2>/dev/null)

if [ -z "$LATEST_COMMIT_TS" ]; then
    echo -e "${RED}[FATAL] 无法获取最新 commit 时间戳（不在 git 仓库中？）${NC}"
    exit 2
fi

echo "最新 commit: ${LATEST_COMMIT_HASH} ${LATEST_COMMIT_MSG}"
echo "  时间戳: $(date -d "@${LATEST_COMMIT_TS}" '+%Y-%m-%d %H:%M:%S %Z' 2>/dev/null || date -r "${LATEST_COMMIT_TS}" '+%Y-%m-%d %H:%M:%S %Z' 2>/dev/null || echo "epoch=${LATEST_COMMIT_TS}")"
echo ""

# ====== 2. 检查 docker 可用 ======
if ! command -v docker &>/dev/null; then
    echo -e "${RED}[FATAL] docker 不可用${NC}"
    exit 2
fi

# ====== 3. 逐个检查镜像时间戳 ======
stale_count=0
total_checked=0

for svc in "${SERVICES[@]}"; do
    # 检查容器是否在运行
    container_state=$(docker inspect --format='{{.State.Status}}' "$svc" 2>/dev/null)
    if [ "$?" -ne 0 ] || [ "$container_state" != "running" ]; then
        echo -e "${YELLOW}[SKIP]${NC} ${svc}: 容器未运行 (state=${container_state:-not_found})"
        continue
    fi

    # 获取镜像 Created 时间戳（ISO 8601 格式）
    image_created=$(docker inspect --format='{{.Created}}' "$svc" 2>/dev/null)
    if [ -z "$image_created" ]; then
        echo -e "${RED}[ERROR]${NC} ${svc}: 无法获取镜像 Created 时间"
        stale_count=$((stale_count + 1))
        total_checked=$((total_checked + 1))
        continue
    fi

    # 转换为 epoch 秒（兼容 GNU date 和 BSD date）
    # Docker 的 Created 格式：2026-09-20T02:00:00.000000000Z
    if date --version &>/dev/null 2>&1; then
        # GNU date
        image_epoch=$(date -d "$image_created" '+%s' 2>/dev/null)
    else
        # BSD date (macOS) — 需要截掉纳秒部分
        clean_ts=$(echo "$image_created" | sed 's/\.[0-9]*Z$/Z/' | sed 's/T/ /' | sed 's/Z$//')
        image_epoch=$(date -j -f "%Y-%m-%d %H:%M:%S" "$clean_ts" '+%s' 2>/dev/null)
    fi

    if [ -z "$image_epoch" ] || [ "$image_epoch" -eq 0 ] 2>/dev/null; then
        echo -e "${RED}[ERROR]${NC} ${svc}: 无法解析镜像时间: ${image_created}"
        stale_count=$((stale_count + 1))
        total_checked=$((total_checked + 1))
        continue
    fi

    # 获取镜像 ID（短格式，便于追踪）
    image_id=$(docker inspect --format='{{.Image}}' "$svc" 2>/dev/null | cut -c8-19)
    image_created_human=$(date -d "$image_created" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "$image_created")

    total_checked=$((total_checked + 1))

    if [ "$image_epoch" -gt "$LATEST_COMMIT_TS" ]; then
        echo -e "${GREEN}[OK]${NC}   ${svc}: 镜像比 commit 新"
        echo "      镜像: ${image_id} 构建于 ${image_created_human}"
    else
        echo -e "${RED}[STALE]${NC} ${svc}: 镜像比 commit 旧!"
        echo "      镜像: ${image_id} 构建于 ${image_created_human}"
        echo "      commit: ${LATEST_COMMIT_HASH} 在 $(date -d "@${LATEST_COMMIT_TS}" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "epoch=${LATEST_COMMIT_TS}")"
        echo "      修复: docker compose -f deploy/docker-compose.apps.yml build ${svc}"
        stale_count=$((stale_count + 1))
    fi
done

# ====== 4. 汇总 ======
echo ""
echo "=== 汇总 ==="
if [ "$total_checked" -eq 0 ]; then
    echo -e "${YELLOW}无运行中的 emotion-echo 容器（全部 SKIP）${NC}"
    echo "请先启动容器:"
    echo "  docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d"
    exit 2
fi

if [ "$stale_count" -eq 0 ]; then
    echo -e "${GREEN}GREEN: 全部 ${total_checked} 个镜像均比最新 commit 新${NC}"
    exit 0
else
    echo -e "${RED}RED: ${stale_count}/${total_checked} 个镜像陈旧（stale）${NC}"
    echo ""
    echo "修复方法:"
    echo "  1. 重建所有陈旧镜像:"
    echo "     docker compose -f deploy/docker-compose.apps.yml build"
    echo "  2. 重建单个镜像:"
    echo "     docker compose -f deploy/docker-compose.apps.yml build <service-name>"
    echo "  3. 重建并重启:"
    echo "     docker compose -f deploy/docker-compose.apps.yml up -d --build"
    echo ""
    echo "依据: E2E-F-70 — 先改代码不重建镜像就验收会得到错误结论"
    exit 1
fi

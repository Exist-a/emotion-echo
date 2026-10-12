#!/usr/bin/env bash
#
# scripts/check_image_freshness.sh — 镜像新鲜度校验（E2E-F-70 机械化）
#
# 目的：验证正在运行的 emotion-echo 服务容器的**镜像构建时间**晚于
#       **该服务源码路径**的最近一次 git commit 时间戳。
#
# 背景：E2E-F-70 记录了一个真实的验收陷阱——先改代码、不重建镜像就直接验收，
#       得到"改前"的结论（无论结论是"已修复"还是"仍坏"）。
#       本脚本机械地拦截此类情况。
#
# 🔴 判据修正（E2E-F-215，2026-10-12）：
#   原实现有两处缺陷，使其在**非产物提交**（只改 docs/scripts，或只改注释）上恒红：
#     ① 读**容器**的 `{{.Created}}`（容器创建时间）却声称"镜像构建于"——取错对象。
#        → 改为读**镜像**的 Created（`docker image inspect`）。
#     ② 拿**全局最新 commit** 比 ⇒ 任何 docs/scripts 提交都会让全部服务判 STALE。
#        → 改为按**该服务源码路径**（+ 共享包 emotion-echo-shared）取最近一次 commit。
#   注：服务源码本身的改动（含注释）仍会触发 STALE —— 这是**有意的**：源码变了就应重建。
#
# 前置：docker 容器正在运行
#
# 退出码：
#   0 = 所有被测镜像均比各自源码最新 commit 新
#   1 = 至少一个镜像陈旧（stale）
#   2 = 运行时错误（docker 不可用 / 无容器 / git 不可用）
#
# 用法：
#   bash scripts/check_image_freshness.sh
#
# 依据：discovered-unresolved.md E2E-F-70 / E2E-F-215、RUNBOOK.md

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

# 服务 → 其构建输入的 git 路径集合（与 smoke_data_layer.py §8 同口径）。
# 前端不依赖 Go 共享包；其余服务都 import emotion-echo-shared。
svc_src_paths() {
    case "$1" in
        emotion-echo-web) echo "emotion-echo-web" ;;
        *)                echo "$1 emotion-echo-shared" ;;
    esac
}

echo "=== E2E-F-70 镜像新鲜度校验（判据：镜像构建时间 ≥ 该服务源码最近 commit）==="
echo ""

# ====== 1. 前置检查 ======
if ! command -v git &>/dev/null; then
    echo -e "${RED}[FATAL] git 不可用${NC}"
    exit 2
fi
if ! command -v docker &>/dev/null; then
    echo -e "${RED}[FATAL] docker 不可用${NC}"
    exit 2
fi

# ====== 2. 逐个检查 ======
stale_count=0
total_checked=0

for svc in "${SERVICES[@]}"; do
    src_paths=$(svc_src_paths "$svc")

    # 容器在运行？
    container_state=$(docker inspect --format='{{.State.Status}}' "$svc" 2>/dev/null)
    if [ "$?" -ne 0 ] || [ "$container_state" != "running" ]; then
        echo -e "${YELLOW}[SKIP]${NC} ${svc}: 容器未运行 (state=${container_state:-not_found})"
        continue
    fi

    # 该服务源码路径的最近一次 commit
    src_ts=$(git log -1 --format='%ct' -- $src_paths 2>/dev/null)
    src_hash=$(git log -1 --format='%h' -- $src_paths 2>/dev/null)
    if [ -z "$src_ts" ]; then
        echo -e "${YELLOW}[SKIP]${NC} ${svc}: 源码路径无 commit 记录 (${src_paths})"
        continue
    fi

    # 容器所用**镜像**引用 → 镜像构建时间（不是容器创建时间）
    image_ref=$(docker inspect --format='{{.Image}}' "$svc" 2>/dev/null)
    if [ -z "$image_ref" ]; then
        echo -e "${RED}[ERROR]${NC} ${svc}: 无法获取容器所用镜像 ID"
        stale_count=$((stale_count + 1)); total_checked=$((total_checked + 1))
        continue
    fi
    image_created=$(docker image inspect --format='{{.Created}}' "$image_ref" 2>/dev/null)
    if [ -z "$image_created" ]; then
        echo -e "${RED}[ERROR]${NC} ${svc}: 无法获取镜像 Created 时间"
        stale_count=$((stale_count + 1)); total_checked=$((total_checked + 1))
        continue
    fi

    # epoch 转换（GNU date / BSD date 兼容）
    if date --version &>/dev/null 2>&1; then
        image_epoch=$(date -d "$image_created" '+%s' 2>/dev/null)
        src_human=$(date -d "@${src_ts}" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "epoch=${src_ts}")
    else
        clean_ts=$(echo "$image_created" | sed 's/\.[0-9]*Z$/Z/' | sed 's/T/ /' | sed 's/Z$//')
        image_epoch=$(date -j -f "%Y-%m-%d %H:%M:%S" "$clean_ts" '+%s' 2>/dev/null)
        src_human=$(date -r "${src_ts}" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "epoch=${src_ts}")
    fi

    if [ -z "$image_epoch" ] || [ "$image_epoch" -eq 0 ] 2>/dev/null; then
        echo -e "${RED}[ERROR]${NC} ${svc}: 无法解析镜像时间: ${image_created}"
        stale_count=$((stale_count + 1)); total_checked=$((total_checked + 1))
        continue
    fi

    image_id=$(echo "$image_ref" | cut -c8-19)
    image_created_human=$(date -d "$image_created" '+%Y-%m-%d %H:%M:%S' 2>/dev/null || echo "$image_created")

    total_checked=$((total_checked + 1))

    if [ "$image_epoch" -ge "$src_ts" ]; then
        echo -e "${GREEN}[OK]${NC}   ${svc}: 镜像 ≥ 源码 commit ${src_hash}"
        echo "      镜像: ${image_id} 构建于 ${image_created_human}"
    else
        echo -e "${RED}[STALE]${NC} ${svc}: 镜像早于源码 commit!"
        echo "      镜像: ${image_id} 构建于 ${image_created_human}"
        echo "      源码 commit: ${src_hash} 在 ${src_human}（路径: ${src_paths}）"
        echo "      修复: docker compose -f deploy/docker-compose.apps.yml build ${svc}"
        stale_count=$((stale_count + 1))
    fi
done

# ====== 3. 汇总 ======
echo ""
echo "=== 汇总 ==="
if [ "$total_checked" -eq 0 ]; then
    echo -e "${YELLOW}无运行中的 emotion-echo 容器（全部 SKIP）${NC}"
    echo "请先启动容器:"
    echo "  docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d"
    exit 2
fi

if [ "$stale_count" -eq 0 ]; then
    echo -e "${GREEN}GREEN: 全部 ${total_checked} 个镜像均比各自源码 commit 新${NC}"
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
    echo "依据: E2E-F-70 / E2E-F-215 — 先改代码不重建镜像就验收会得到错误结论"
    exit 1
fi

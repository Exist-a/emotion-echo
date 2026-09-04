#!/usr/bin/env bash
# build_dev_images.sh — 镜像 build 重试脚本
#
# 解决的问题（2026-09-04 实战踩到）：
#   1. docker.io 偶发 401 "pull access denied / insufficient_scope" — 匿名拉取限流
#   2. alpine / golang / python 基础镜像首次拉被拒 — 重试一次通常就过
#   3. compose build 输出截断看不到完整错误 — 用本脚本聚合
#
# 用法：
#   bash scripts/build_dev_images.sh                    # 全量 build
#   bash scripts/build_dev_images.sh web-bff ai-svc     # 只 build 指定 svc
#   BASE_IMAGES="alpine:3.19 golang:1.26-alpine python:3.12-slim" \
#     bash scripts/build_dev_images.sh                   # 自定义预拉列表
#
# 退出码：0 = 全部成功；1 = 至少 1 个 build 失败

set -euo pipefail

# 切到 deploy 目录（compose 必需）
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/../deploy"

# 6 业务 svc + llm-service（AI profile fer/sensevoice/xtts 是可选 + image 私有，跳过）
ALL_SVCS=(emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
          emotion-echo-assessment-svc emotion-echo-ai-svc emotion-echo-web-bff \
          emotion-llm-service)

# 用户参数：指定要 build 的 svc
if [ $# -gt 0 ]; then
  SVCS=("$@")
else
  SVCS=("${ALL_SVCS[@]}")
fi

# 默认要预拉的基础镜像（避免 build 时拉到一半被 docker.io 401）
BASE_IMAGES="${BASE_IMAGES:-alpine:3.19 golang:1.26-alpine python:3.12-slim}"

# 预拉配置：每个 base image 重试 N 次
MAX_RETRY="${MAX_RETRY:-3}"
RETRY_DELAY="${RETRY_DELAY:-3}"

pull_with_retry() {
  local img="$1"
  local attempt=1
  while [ $attempt -le $MAX_RETRY ]; do
    if docker pull "$img" 2>&1 | tail -1 | grep -qE "Downloaded|is up to date"; then
      return 0
    fi
    echo "  [retry $attempt/$MAX_RETRY] pull $img failed, sleep ${RETRY_DELAY}s"
    sleep "$RETRY_DELAY"
    attempt=$((attempt + 1))
  done
  return 1
}

echo "=== Pre-pull base images ==="
for img in $BASE_IMAGES; do
  echo "  pulling $img ..."
  if ! pull_with_retry "$img"; then
    echo "  [WARN] $img pull failed after $MAX_RETRY retries; build may still work if cached"
  fi
done

# Build 阶段：每个 svc 重试
echo
echo "=== Build services ==="
echo "  targets: ${SVCS[*]}"
fail=0
for svc in "${SVCS[@]}"; do
  attempt=1
  while [ $attempt -le $MAX_RETRY ]; do
    echo
    echo "--- [build $attempt/$MAX_RETRY] $svc ---"
    if docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml build "$svc" 2>&1 | tee /tmp/build_${svc}.log | tail -3; then
      if grep -q "^ Image .* Built$" /tmp/build_${svc}.log; then
        echo "  [OK] $svc built"
        break
      fi
    fi
    echo "  [retry $attempt/$MAX_RETRY] $svc build failed, sleep ${RETRY_DELAY}s"
    sleep "$RETRY_DELAY"
    attempt=$((attempt + 1))
  done
  if [ $attempt -gt $MAX_RETRY ]; then
    echo "  [FAIL] $svc build failed after $MAX_RETRY retries"
    fail=1
  fi
done

if [ $fail -ne 0 ]; then
  echo
  echo "=== Build summary: SOME FAILED ==="
  exit 1
fi
echo
echo "=== Build summary: ALL OK ==="
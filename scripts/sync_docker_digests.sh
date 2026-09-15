#!/usr/bin/env bash
#
# scripts/sync_docker_digests.sh — Round 4.7 PR-3 digest sync
#
# 用途：从 Docker Hub registry API 查询所有 base image 当前 digest，
#       写回 Dockerfile.digests.lock 文件。
#
# 需要：网络可达 registry-1.docker.io（或通过 docker.m.daocloud.io 镜像）。
#
# 用法：
#   bash scripts/sync_docker_digests.sh

set -euo pipefail

LOCK_FILE="Dockerfile.digests.lock"

# 镜像列表（image:tag → env var 名）
declare -A IMAGES=(
    ["alpine:3.19"]="ALPINE_3_19_DIGEST"
    ["golang:1.26-alpine"]="GOLANG_1_26_ALPINE_DIGEST"
    ["python:3.10-slim"]="PYTHON_3_10_SLIM_DIGEST"
    ["python:3.12-slim"]="PYTHON_3_12_SLIM_DIGEST"
    ["node:20-alpine"]="NODE_20_ALPINE_DIGEST"
    ["ubuntu:22.04"]="UBUNTU_22_04_DIGEST"
)

# 任一镜像 registry endpoint（CN 镜像可能可达）
REGISTRY="${DOCKER_REGISTRY:-registry-1.docker.io}"

# 先获取 anonymous token
TOKEN_URL="https://auth.${REGISTRY#registry-1.}/token?service=${REGISTRY}&scope=repository:library/alpine:pull"
TOKEN=$(curl -s --max-time 10 "${TOKEN_URL}" | jq -r '.token // empty' 2>/dev/null || true)

if [[ -z "${TOKEN}" ]]; then
    echo "WARN: cannot fetch anonymous token from ${TOKEN_URL}"
    echo "      (offline mode — script will skip digests marked [UPDATE REQUIRED])"
    echo ""
fi

new_digests=()

for image_tag in "${!IMAGES[@]}"; do
    var="${IMAGES[$image_tag]}"
    image="${image_tag%:*}"
    tag="${image_tag#*:}"
    echo "Fetching digest for ${image_tag} ..."
    digest=$(curl -s --max-time 10 \
        -H "Accept: application/vnd.docker.distribution.manifest.v2+json" \
        -H "Authorization: Bearer ${TOKEN}" \
        "https://${REGISTRY}/v2/library/${image}/manifests/${tag}" \
        | jq -r '.config.digest // empty' 2>/dev/null || true)
    if [[ -z "${digest}" ]]; then
        echo "  WARN: digest fetch failed (offline?)"
        continue
    fi
    echo "  ${var}=${digest}"
    new_digests+=("${var}=${digest}")
done

# 写回 lock 文件（仅替换占位 000...000 / UPDATE REQUIRED）
for entry in "${new_digests[@]}"; do
    var="${entry%%=*}"
    val="${entry#*=}"
    # 仅当文件中仍是 000..000 时替换（避免覆盖已验证的 digest）
    sed -i.bak "s|^${var}=sha256:0\{64\}.*|${var}=${val}  # synced $(date +%Y-%m-%d)|" "$LOCK_FILE"
done
rm -f "${LOCK_FILE}.bak"

echo ""
echo "Updated $LOCK_FILE. Run scripts/check_docker_digests.sh to verify."

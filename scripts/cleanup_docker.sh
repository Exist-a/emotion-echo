#!/usr/bin/env bash
# scripts/cleanup_docker.sh
#
# CHORE-0 定期清理脚本（Stage 58 引入）
#
# 释放目标：
#   - 旧版本镜像（项目已切换到新版本但旧 tag 仍占空间）
#   - 悬空镜像（多阶段构建的中间层）
#   - 已停止容器（保留卷数据，避免误删 Postgres/MinIO 数据）
#   - 未使用的自定义网络（释放 iptables 规则）
#
# 安全网：
#   - 删前备份 docker images 清单到 docker-images-before.txt
#   - 删后立即打印 docker system df 供对比
#   - 不删 volume（数据库 / MinIO 数据）
#   - 不删 compose 当前引用的镜像
#
# 用法：
#   bash scripts/cleanup_docker.sh
#
# 风险：
#   - 如果未来 compose 引用的镜像 tag 变了，旧的"未引用"镜像会被认为是悬空
#     ——请定期跑完后核对 `docker compose config` 能正常启动 dev 链路

set -uo pipefail

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[$(date '+%H:%M:%S')]${NC} $*"; }
warn() { echo -e "${YELLOW}[$(date '+%H:%M:%S')]${NC} $*"; }
err() { echo -e "${RED}[$(date '+%H:%M:%S')]${NC} $*"; }

# === 0. 备份镜像清单 ===
log "=== Step 0: Backup current image list to docker-images-before.txt ==="
docker images > docker-images-before.txt
echo "  Lines: $(wc -l < docker-images-before.txt)"

# === 1. 记录清理前磁盘占用 ===
log "=== Step 1: Disk usage BEFORE cleanup ==="
docker system df

# === 2. 删旧版本镜像（按需更新此列表） ===
log "=== Step 2: Remove old version images ==="

# 项目用 nacos v2.4.3，v2.3.2 是 Stage 41 go-zero 移除前残留
if docker images --format '{{.Repository}}:{{.Tag}}' | grep -q '^nacos/nacos-server:v2.3.2$'; then
  log "  Removing nacos/nacos-server:v2.3.2 ..."
  docker rmi nacos/nacos-server:v2.3.2 || warn "  Failed (may be in use)"
else
  log "  nacos/nacos-server:v2.3.2 not present (already cleaned)"
fi

# chat-svc:tzfix 是 Stage 42 TZ 修复时的临时 tag，已被 v0.1.0 取代
if docker images --format '{{.Repository}}:{{.Tag}}' | grep -q '^emotion-echo/chat-svc:tzfix$'; then
  log "  Removing emotion-echo/chat-svc:tzfix ..."
  docker rmi emotion-echo/chat-svc:tzfix || warn "  Failed (may be in use)"
else
  log "  emotion-echo/chat-svc:tzfix not present (already cleaned)"
fi

# === 3. 删全部 Exited 容器（保留卷） ===
log "=== Step 3: Prune stopped containers (volumes preserved) ==="
docker container prune -f

# === 4. 删悬空镜像 ===
log "=== Step 4: Prune dangling images ==="
docker image prune -f

# === 5. 删未使用的自定义网络 ===
log "=== Step 5: Prune unused networks ==="
docker network prune -f

# === 6. 记录清理后磁盘占用 ===
log "=== Step 6: Disk usage AFTER cleanup ==="
docker system df

# === 7. 验证：列出最终镜像 ===
log "=== Step 7: Final image list ==="
docker images --format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}\t{{.CreatedSince}}"

log "=== Cleanup complete ==="
log "Backup saved to: docker-images-before.txt"
log "Next cleanup recommended in 30 days"
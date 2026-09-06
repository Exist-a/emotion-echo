#!/usr/bin/env bash
# scripts/check_minio_health.sh — Sprint 1 PR-4a MinIO 健康检查契约
#
# 用途：dev compose 启动后，验证 MinIO 服务可达 + avatars bucket 已建。
#       是 PR-4 落地证明（avatar 上传必须先有 MinIO）。
#
# 契约断言：
#   1. emotion-echo-minio 容器 running
#   2. localhost:9000/health/live 返 200 (MinIO liveness probe)
#   3. localhost:9001/ (MinIO 控制台) 可达 (dev 默认账号 minioadmin/minioadmin)
#   4. avatars bucket 存在（通过 mc ls 或 mc stat）
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
#
# 用法：bash scripts/check_minio_health.sh

set -uo pipefail

fail_count=0
log()  { echo "[minio] $*"; }
err()  { echo "[minio] FAIL: $*" >&2; fail_count=$((fail_count + 1)); }

# ---------- 前置：docker compose 已 up ----------
MINIO_CONTAINER="emotion-echo-minio"
if ! docker inspect "$MINIO_CONTAINER" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
  err "container $MINIO_CONTAINER 未运行（先 docker compose up -d）"
  exit 1
fi

# ---------- 契约 1: 容器 running ----------
log "=== contract 1: $MINIO_CONTAINER running ==="
state=$(docker inspect "$MINIO_CONTAINER" --format '{{.State.Status}}' 2>/dev/null)
if [ "$state" = "running" ]; then
  log "  OK ($MINIO_CONTAINER state=$state)"
else
  err "$MINIO_CONTAINER state=$state (期望 running)"
fi

# ---------- 契约 2: liveness probe 200 ----------
log ""
log "=== contract 2: MinIO liveness (localhost:9000/minio/health/live) ==="
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://localhost:9000/minio/health/live" 2>/dev/null || echo "000")
if [ "$code" = "200" ]; then
  log "  OK (HTTP 200)"
else
  err "MinIO /minio/health/live 返 HTTP $code (期望 200)"
fi

# ---------- 契约 3: console 可达 ----------
log ""
log "=== contract 3: MinIO console (localhost:9001) ==="
code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 5 "http://localhost:9001/" 2>/dev/null || echo "000")
if [ "$code" = "200" ] || [ "$code" = "301" ] || [ "$code" = "302" ]; then
  log "  OK (HTTP $code，dev 默认账号 minioadmin/minioadmin)"
else
  err "MinIO console 返 HTTP $code (期望 200/301/302)"
fi

# ---------- 契约 4: avatars bucket 存在 ----------
log ""
log "=== contract 4: avatars bucket 已建 ==="
# init 容器一次性（Exited 0）不能再 exec；直接用 mc 容器验证
# mc 镜像 entrypoint 是 mc，用 --entrypoint bash 覆盖
mc_output=$(docker run --rm --network emotion-echo_app-network \
  --entrypoint bash quay.io/minio/mc:latest -c \
  "mc alias set dev http://emotion-echo-minio:9000 minioadmin minioadmin >/dev/null 2>&1 && mc ls dev/" 2>&1)
if echo "$mc_output" | grep -q 'avatars'; then
  log "  OK (avatars bucket 存在)"
else
  err "avatars bucket 不存在（emotion-echo-minio-init 应建 dev/avatars）；mc output: $mc_output"
fi

echo ""
echo "============================================="
echo "fail_count=$fail_count"
if [ "$fail_count" -eq 0 ]; then
  echo "[OK] MinIO 闭环全 PASS"
  exit 0
else
  echo "[FAIL] MinIO 闭环失败"
  exit 1
fi
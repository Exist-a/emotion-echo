#!/usr/bin/env bash
# scripts/smoke_upload_minio.sh — Stage 58 PR-UP-3 §契约 8
#
# 用途：dev compose 启动后，验证通用上传链路通：
#   1. APISIX 路由 POST /api/v1/uploads/:kind 可达（200/4xx）
#   2. POST /api/v1/uploads/image 上传 1x1 PNG 应返 200 + 非空 url
#   3. 返回的 url 在浏览器/curl 可达（MinIO bucket 设置正确）
#   4. mc ls emotion-echo-minio:avatars/uploads/<uid>-* 看到新文件
#
# 契约断言（PR-UP-3 §契约 8）：
#   1. 上传 HTTP 200
#   2. 响应 json 含 url 字段且非空
#   3. url 可 HEAD 访问（MinIO anonymous download OK）
#   4. mc ls avatars 看到 uploads/<uid>-*.png
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
#
# 用法：bash scripts/smoke_upload_minio.sh

set -uo pipefail

APISIX_URL="${APISIX_URL:-http://localhost:19080}"
USER_ID="${USER_ID:-1}"  # APISIX dev 默认注入 X-User-Id=1 (seed echo user)
FAIL=0

log() { echo "[upload] $*"; }
err() { echo "[upload] FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

# ---------- 前置：APISIX + BFF + MinIO 都已 up ----------
for c in emotion-echo-apisix emotion-echo-web-bff emotion-echo-minio; do
  if ! docker inspect "$c" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
    err "container $c 未运行（先 docker compose -f infra -f apps -f dev up -d）"
    exit 1
  fi
done

# ---------- 准备：1x1 透明 PNG（最小合法 image） ----------
TMP_IMG=/tmp/smoke_upload.png
# Base64 编码的 1x1 红色 PNG（67 字节）
PNG_B64="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
echo "$PNG_B64" | base64 -d > "$TMP_IMG" 2>/dev/null
if [ ! -s "$TMP_IMG" ]; then
  err "无法生成测试 PNG 到 $TMP_IMG"
  exit 1
fi

# ---------- 契约 1：HTTP 200 + 响应含 url ----------
log "=== 契约 1: POST /api/v1/uploads/image ==="
UPLOAD_RESP=/tmp/upload_resp.json
HTTP_CODE=$(curl -sS -o "$UPLOAD_RESP" -w '%{http_code}' \
  -X POST "$APISIX_URL/api/v1/uploads/image" \
  -H "X-User-Id: $USER_ID" \
  -F "file=@$TMP_IMG;type=image/png" \
  --max-time 30 2>/dev/null || echo "000")

if [ "$HTTP_CODE" = "200" ]; then
  log "[OK  ] upload HTTP 200"
else
  err "upload HTTP $HTTP_CODE（resp=$(head -c 200 "$UPLOAD_RESP" 2>/dev/null)）"
fi

# 解析 url 字段
URL=$(grep -oE '"url"[[:space:]]*:[[:space:]]*"[^"]+"' "$UPLOAD_RESP" 2>/dev/null | head -1 | sed 's/.*"url"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
if [ -z "$URL" ]; then
  err "响应缺 url 字段（resp=$(head -c 200 "$UPLOAD_RESP" 2>/dev/null)）"
else
  log "[OK  ] url 字段存在：$URL"

  # ---------- 契约 2：HEAD url 可达（MinIO bucket 匿名下载） ----------
  log "=== 契约 2: HEAD $URL ==="
  HEAD_CODE=$(curl -sS -o /dev/null -w '%{http_code}' -I "$URL" --max-time 10 2>/dev/null || echo "000")
  if [ "$HEAD_CODE" = "200" ]; then
    log "[OK  ] url HEAD 200 (MinIO anonymous download 配置正确)"
  else
    err "url HEAD $HEAD_CODE（bucket 可能是 private，MinIO console 改 policy 为 download）"
  fi
fi

# ---------- 契约 3：mc ls avatars 看到 uploads/<uid>-* ----------
log "=== 契约 3: mc ls emotion-echo-minio:avatars/uploads/ ==="
# 用 docker exec 调 mc（mc 镜像已在 init container 用过，但 init 跑完退出，重用 init 镜像）
MC_OUTPUT=$(docker run --rm --network emotion-echo_app-network \
  quay.io/minio/mc:latest \
  ls -r emotion-echo-minio/avatars/uploads/ 2>&1 || echo "MC_FAILED")

if echo "$MC_OUTPUT" | grep -qE "uploads/[0-9]+-.*\.png"; then
  log "[OK  ] mc 看到 uploads/<uid>-*.png 文件"
else
  err "mc 未看到 uploads/<uid>-*.png（output: $MC_OUTPUT）"
fi

# ---------- 汇总 ----------
echo
if [ "$FAIL" -gt 0 ]; then
  echo "=== §契约 8 FAIL: $FAIL 项 ==="
  exit 1
fi
echo "=== §契约 8 ALL PASS ==="
exit 0
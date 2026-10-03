#!/usr/bin/env bash
# scripts/smoke_upload_minio.sh — Stage 58 PR-UP-3 §契约 8
#
# 用途：dev compose 启动后，验证通用上传链路通：
#   0. 登录取 JWT（uploads 路由经 APISIX jwt-auth——E2E-27 #2 修复）
#   1. APISIX 路由 POST /api/v1/uploads/:kind 可达（200/4xx）
#   2. POST /api/v1/uploads/image 上传 1x1 PNG 应返 200 + 非空 url
#   3. 返回的 url 在浏览器/curl 可达（MinIO bucket 设置正确）
#   4. mc ls（MC_HOST_ 免 alias）看到 uploads/<uid>-* 新文件
#
# 契约断言（PR-UP-3 §契约 8）：
#   1. 上传 HTTP 200
#   2. 响应 json 含 url 字段且非空
#   3. url 可 HEAD 访问（MinIO anonymous download OK）
#   4. mc ls avatars 看到 uploads/<uid>-*.png
#
# E2E-27 #2 三缺陷修复（2026-10-03 运行时实测，此前三项从未同时真跑通过）：
#   1. 临时文件改 cwd 相对路径——mingw curl 读不到 /tmp（rc26 → HTTP 000000）
#   2. 登录取 Bearer——只带 X-User-Id 被 jwt-auth 拒 401（同 E2E-F-182 型）
#   3. mc 走 MC_HOST_dev 环境变量——docker run 裸 mc 无 alias ⇒ 空输出 rc=0
#      静默假列，契约 4 永远 FAIL
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
#
# 用法：bash scripts/smoke_upload_minio.sh
#   覆盖：APISIX_URL / USER_ID / SMOKE_USER / SMOKE_PASSWORD

set -uo pipefail

APISIX_URL="${APISIX_URL:-http://localhost:19080}"
USER_ID="${USER_ID:-1}"  # 客户端 X-User-Id（APISIX jwt-auth 会以 token 内 uid 为准）
SMOKE_USER="${SMOKE_USER:-smoke_user}"          # Stage 37 数据契约契约账号
SMOKE_PASSWORD="${SMOKE_PASSWORD:-echo123}"     # deploy/db/03-seed-default-users.sql
MINIO_ROOT_USER="${MINIO_ROOT_USER:-minioadmin}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-minioadmin}"
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

# ---------- 准备：1x1 透明 PNG（cwd 相对路径——native curl 读不到 /tmp） ----------
TMP_IMG="./.smoke_upload_minio.png"
UPLOAD_RESP="./.smoke_upload_minio_resp.json"
trap 'rm -f "$TMP_IMG" "$UPLOAD_RESP"' EXIT
# Base64 编码的 1x1 红色 PNG（67 字节）
PNG_B64="iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
echo "$PNG_B64" | base64 -d > "$TMP_IMG" 2>/dev/null
if [ ! -s "$TMP_IMG" ]; then
  err "无法生成测试 PNG 到 $TMP_IMG"
  exit 1
fi

# ---------- 准备：登录取 JWT（uploads 路由经 APISIX jwt-auth） ----------
LOGIN_RESP=$(curl -sS --max-time 10 -X POST "$APISIX_URL/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$SMOKE_USER\",\"password\":\"$SMOKE_PASSWORD\"}" 2>/dev/null || true)
TOKEN=$(echo "$LOGIN_RESP" | grep -oE '"accessToken":"[^"]+' | head -1 | cut -d'"' -f4)
if [ -z "$TOKEN" ]; then
  err "登录失败（user=$SMOKE_USER）：resp=$(echo "$LOGIN_RESP" | head -c 200)"
  exit 1
fi
log "登录成功（$SMOKE_USER，token ${#TOKEN} 字节）"

# ---------- 契约 1：HTTP 200 + 响应含 url ----------
log "=== 契约 1: POST /api/v1/uploads/image ==="
HTTP_CODE=$(curl -sS -o "$UPLOAD_RESP" -w '%{http_code}' \
  -X POST "$APISIX_URL/api/v1/uploads/image" \
  -H "Authorization: Bearer $TOKEN" \
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

  # ---------- 契约 2：HEAD url 可达 ----------
  # E2E-27 M1：url 已是网关相对路径（/api/v1/uploads/file/...，反代端点走
  # jwt-auth）——相对路径经网关 + Bearer HEAD；legacy 绝对地址（存量）直接 HEAD。
  # 可达性语义不变：返回的 url 在浏览器视角必须打得通。
  if [ "${URL#/}" != "$URL" ]; then
    HEAD_TARGET="$APISIX_URL$URL"
    HEAD_AUTH=(-H "Authorization: Bearer $TOKEN")
  else
    HEAD_TARGET="$URL"
    HEAD_AUTH=()
  fi
  log "=== 契约 2: HEAD $HEAD_TARGET ==="
  HEAD_CODE=$(curl -sS -o /dev/null -w '%{http_code}' -I "${HEAD_AUTH[@]}" "$HEAD_TARGET" --max-time 10 2>/dev/null || echo "000")
  if [ "$HEAD_CODE" = "200" ]; then
    log "[OK  ] url HEAD 200 (对象可达——M1 反代端点 / legacy 匿名)"
  else
    err "url HEAD $HEAD_CODE（反代端点应经网关 200；legacy 则检查 bucket 匿名策略）"
  fi
fi

# ---------- 契约 3：mc ls avatars 看到 uploads/<uid>-* ----------
log "=== 契约 3: mc ls dev/avatars/uploads/（MC_HOST_ 免 alias） ==="
# MC_HOST_dev 让全新 mc 容器直接持有连接（免 mc alias set；裸 ls 无 alias 时
# 静默空输出 rc=0，契约 4 永远无法真验——E2E-27 #2 缺陷 3）
MC_OUTPUT=$(docker run --rm --network emotion-echo_app-network \
  -e "MC_HOST_dev=http://${MINIO_ROOT_USER}:${MINIO_ROOT_PASSWORD}@emotion-echo-minio:9000" \
  quay.io/minio/mc:latest \
  ls -r dev/avatars/uploads/ 2>&1 || echo "MC_FAILED")

# 断言**本轮上传对象**在列（URL basename 精确匹配）——E2E-27 运行时修正：
# mc ls 对指定前缀显示的是相对 key（无 "uploads/" 目录段，原 grep 永不匹配）；
# 且裸 uid-pattern 会把任意历史文件当成本轮证据（弱断言），改 basename 只认本轮。
if [ -n "$URL" ] && echo "$MC_OUTPUT" | grep -qF "$(basename "$URL")"; then
  log "[OK  ] mc 看到本轮对象 $(basename "$URL")"
else
  err "mc 未看到本轮对象（url=${URL:-<空>} output: $(echo "$MC_OUTPUT" | head -c 300)）"
fi

# ---------- 汇总 ----------
echo
if [ "$FAIL" -gt 0 ]; then
  echo "=== §契约 8 FAIL: $FAIL 项 ==="
  exit 1
fi
echo "=== §契约 8 ALL PASS ==="
exit 0

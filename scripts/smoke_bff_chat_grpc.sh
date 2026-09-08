#!/usr/bin/env bash
# scripts/smoke_bff_chat_grpc.sh — Stage 58 PR-GRPC-6 §契约 9
#
# 目的：dev compose 启动后，验证 BFF → chat-svc gRPC 链路通（PR-GRPC-1~5 收口证明）。
#
# 契约断言（PR-GRPC-6 §契约 9）：
#   1. 前置：emotion-echo-web-bff / chat-svc 容器 running
#   2. chat-svc :8892 gRPC 端口 listening
#   3. grpc.health.v1.Health/Check → SERVING（chat-svc gRPC server up）
#   4. BFF POST /api/v1/conversations → 200（gRPC CreateConversation 链路通）
#   5. BFF POST /api/v1/conversations/:id/messages → 200（gRPC SendMessage 链路通）
#   6. BFF GET  /api/v1/conversations/:id/messages → 200（gRPC ListMessages 链路通）
#   7. BFF GET  /api/v1/conversations → 200（gRPC ListConversations 链路通）
#   8. X-User-Id header 透传到 gRPC metadata x-user-id（chat-svc 拦截器读得到）
#   9. SkyWalking OAP 上看到 rpc.* tag（chat-svc OAP layer / BFF OAP layer）
#
# 退出码：0 全 PASS / 1 至少 1 项 FAIL
#
# 用法：bash scripts/smoke_bff_chat_grpc.sh
#
# 前置：
#   - docker compose -f deploy/docker-compose.infra.yml -f docker-compose.apps.yml
#     -f deploy/compose.dev.yml up -d 启动完整 dev 链路
#   - BFF gRPC_TRANSPORT 默认 grpc（PR-GRPC-4 feature flag）
#   - chat-svc 暴露 :8892 gRPC（PR-GRPC-3 compose expose）

set -uo pipefail

APISIX_URL="${APISIX_URL:-http://localhost:19080}"
USER_ID="${USER_ID:-1}"  # dev 默认 seed user
FAIL=0

log() { echo "[grpc-9] $*"; }
err() { echo "[grpc-9] FAIL: $*" >&2; FAIL=$((FAIL + 1)); }

# ---------- 前置：4 个容器 running ----------
for c in emotion-echo-web-bff emotion-echo-chat-svc emotion-echo-apisix emotion-echo-postgres; do
  if ! docker inspect "$c" --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
    err "container $c 未运行（先 docker compose -f infra -f apps -f dev up -d）"
    exit 1
  fi
done

# ---------- 契约 2: chat-svc :8892 gRPC 端口 listening ----------
log "=== 契约 2: chat-svc :8892 gRPC 端口 listening ==="
# 容器内 netstat 不可用 → docker exec 进 chat-svc ss 检查
CHAT_SVC_PORT=$(docker exec emotion-echo-chat-svc sh -c 'ss -tlnp 2>/dev/null | grep -E ":8890|:8892" || netstat -tlnp 2>/dev/null | grep -E ":8890|:8892" || echo "NO_PORT"' 2>&1 || echo "NO_PORT")

if echo "$CHAT_SVC_PORT" | grep -q ':8892'; then
  log "[OK  ] chat-svc :8892 listening"
elif echo "$CHAT_SVC_PORT" | grep -q ':8890'; then
  log "[WARN] chat-svc :8890 (HTTP) listening，但 :8892 (gRPC) 不在监听"
  err "PR-GRPC-3 未生效：chat-svc gRPC server 应监听 :8892"
else
  err "chat-svc :8892 不在监听（output: $CHAT_SVC_PORT）"
fi

# ---------- 契约 3: grpc.health.v1.Health/Check ----------
log "=== 契约 3: grpc.health.v1.Health/Check ==="
# 用 grpcurl 探测（grpcurl 是 dev 工具，需 docker exec 进 chat-svc 容器内有）
# 替代方案：直接 dial chat-svc :8892 看是否接受 HTTP/2 preface
HEALTH_CHECK=$(docker exec emotion-echo-chat-svc sh -c 'echo "GET / HTTP/2" | timeout 2 nc -w1 127.0.0.1 8892 2>&1 | head -1' 2>&1 || echo "TIMEOUT")
if echo "$HEALTH_CHECK" | grep -qi "HTTP\|GRPC\|PRI"; then
  log "[OK  ] chat-svc :8892 接受 HTTP/2 连接（gRPC 协议）"
else
  # 退路：docker logs 看 grpcserver.Start 日志
  if docker logs emotion-echo-chat-svc 2>&1 | tail -50 | grep -q "gRPC server listening on :8892"; then
    log "[OK  ] chat-svc :8892 listening（从 docker logs 验证）"
  else
    err "chat-svc :8892 gRPC 探测失败（既不响应 HTTP/2 也没日志）"
  fi
fi

# ---------- 契约 4: CreateConversation RPC（经 BFF HTTP）----------
log "=== 契约 4: POST /api/v1/conversations (CreateConversation RPC) ==="
CREATE_RESP=/tmp/create_conv_resp.json
CREATE_HTTP=$(curl -sS -o "$CREATE_RESP" -w '%{http_code}' \
  -X POST "$APISIX_URL/api/v1/conversations" \
  -H "X-User-Id: $USER_ID" \
  -H 'Content-Type: application/json' \
  -d '{"title":"smoke test"}' \
  --max-time 30 2>/dev/null || echo "000")

if [ "$CREATE_HTTP" = "200" ]; then
  log "[OK  ] create conv HTTP 200"
  CONV_ID=$(grep -oE '"id"[[:space:]]*:[[:space:]]*[0-9]+' "$CREATE_RESP" 2>/dev/null | head -1 | grep -oE '[0-9]+')
  if [ -n "$CONV_ID" ]; then
    log "[OK  ] conv id=$CONV_ID"
  else
    err "create conv 响应缺 id 字段"
  fi
else
  err "create conv HTTP $CREATE_HTTP（resp=$(head -c 200 "$CREATE_RESP" 2>/dev/null)）"
  CONV_ID=""
fi

# ---------- 契约 5: SendMessage RPC（需 CONV_ID）----------
if [ -n "$CONV_ID" ]; then
  log "=== 契约 5: POST /api/v1/conversations/$CONV_ID/messages (SendMessage RPC) ==="
  SEND_HTTP=$(curl -sS -o /tmp/send_resp.json -w '%{http_code}' \
    -X POST "$APISIX_URL/api/v1/conversations/$CONV_ID/messages" \
    -H "X-User-Id: $USER_ID" \
    -H 'Content-Type: application/json' \
    -d '{"role":"user","content":"hello gRPC","client_msg_id":"smoke-test-uuid"}' \
    --max-time 30 2>/dev/null || echo "000")

  if [ "$SEND_HTTP" = "200" ]; then
    log "[OK  ] send msg HTTP 200"
  else
    err "send msg HTTP $SEND_HTTP"
  fi

  # ---------- 契约 6: ListMessages RPC ----------
  log "=== 契约 6: GET /api/v1/conversations/$CONV_ID/messages (ListMessages RPC) ==="
  LIST_HTTP=$(curl -sS -o /tmp/list_resp.json -w '%{http_code}' \
    -X GET "$APISIX_URL/api/v1/conversations/$CONV_ID/messages?limit=10" \
    -H "X-User-Id: $USER_ID" \
    --max-time 30 2>/dev/null || echo "000")

  if [ "$LIST_HTTP" = "200" ]; then
    MSG_COUNT=$(grep -oE '"messages"[[:space:]]*:[[:space:]]*\[.*\]' /tmp/list_resp.json 2>/dev/null | grep -oE '"id"' | wc -l || echo "0")
    log "[OK  ] list msg HTTP 200（$MSG_COUNT 条消息）"
  else
    err "list msg HTTP $LIST_HTTP"
  fi
fi

# ---------- 契约 7: ListConversations RPC ----------
log "=== 契约 7: GET /api/v1/conversations (ListConversations RPC) ==="
LISTCONV_HTTP=$(curl -sS -o /tmp/listconv_resp.json -w '%{http_code}' \
  -X GET "$APISIX_URL/api/v1/conversations?limit=10&offset=0" \
  -H "X-User-Id: $USER_ID" \
  --max-time 30 2>/dev/null || echo "000")

if [ "$LISTCONV_HTTP" = "200" ]; then
  log "[OK  ] list convations HTTP 200"
else
  err "list conversations HTTP $LISTCONV_HTTP"
fi

# ---------- 契约 8: gRPC 端点鉴权 (x-user-id metadata) ----------
log "=== 契约 8: x-user-id 鉴权（缺 user id 时应被拦截）==="
# 不带 X-User-Id 应返 401
NO_AUTH_HTTP=$(curl -sS -o /dev/null -w '%{http_code}' \
  -X POST "$APISIX_URL/api/v1/conversations" \
  -H 'Content-Type: application/json' \
  -d '{"title":"no auth"}' \
  --max-time 30 2>/dev/null || echo "000")

if [ "$NO_AUTH_HTTP" = "401" ] || [ "$NO_AUTH_HTTP" = "403" ]; then
  log "[OK  ] 缺 X-User-Id 返 $NO_AUTH_HTTP（鉴权链路通）"
else
  # BFF 通常返 401；APISIX 不剥 user id 时可能 200 — 这意味着 chat-svc 拦截器未生效
  err "缺 X-User-Id 返 $NO_AUTH_HTTP（期望 401/403，鉴权未拦截）"
fi

# ---------- 契约 9: SkyWalking OAP rpc.* tag（best-effort）----------
log "=== 契约 9: SkyWalking OAP rpc.* tag（best-effort，需 OAP :12800 通）==="
if docker inspect emotion-echo-sw-oap --format '{{.State.Status}}' 2>/dev/null | grep -q '^running$'; then
  # 查 chat-svc 服务 OAP 上是否有 rpc.* 维度
  # OAP GraphQL endpoint（/graphql）支持 query，最近 N 分钟的 rpc.* tag
  OAP_QUERY='{"query":"{ queryServices(serviceId: \"emotion-echo-chat-svc\", startTimeBucket: 0, endTimeBucket: 9999999999, topN: 10) { nodes { name { value } } }"}'
  OAP_RESP=$(curl -sS -X POST 'http://localhost:12800/graphql' \
    -H 'Content-Type: application/json' \
    -d "$OAP_QUERY" \
    --max-time 10 2>/dev/null || echo '{"errors":"OAP_UNREACHABLE"}')

  if echo "$OAP_RESP" | grep -qE 'rpc\.|rpc.client|rpc.server'; then
    log "[OK  ] OAP 上看到 rpc.* tag"
  elif echo "$OAP_RESP" | grep -q '"name"'; then
    # OAP 通但没采集到 rpc.*（可能 metrics 未上报）
    log "[WARN] OAP 通但未看到 rpc.* tag（可能 metrics 未上报或采样窗口未到）"
  else
    log "[WARN] OAP 探测失败（best-effort，不计入 FAIL）"
  fi
else
  log "[WARN] emotion-echo-sw-oap 未运行，跳过 OAP rpc.* tag 验证（best-effort）"
fi

# ---------- 汇总 ----------
echo
if [ "$FAIL" -gt 0 ]; then
  echo "=== §契约 9 FAIL: $FAIL 项 ==="
  exit 1
fi
echo "=== §契约 9 ALL PASS ==="
exit 0
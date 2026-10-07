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
#   8. 缺身份调用受保护端点 → 401（由网关 jwt-auth 承担；BFF 侧信任链见 E2E-29 D-47）
#
# E2E-29 F-182 修复（2026-10-07）：契约 4~7 原只带 `X-User-Id` 不带 `Authorization: Bearer`，
# 而 APISIX 的 catch-all 路由自 jwt-auth 落地起就要求令牌 ⇒ 该脚本**恒 4/7 FAIL**（编写于
# PR-GRPC-6 时代，未随鉴权要求补登录取 token 步骤）。现先 login 取 Bearer 再调受保护端点。
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

# ---------- 契约 1.5: 登录取 Bearer（F-182：受保护端点必须带令牌）----------
log "=== 契约 1.5: POST /api/v1/auth/login 取 accessToken ==="
LOGIN_RESP=$(curl -sS -X POST "$APISIX_URL/api/v1/auth/login"   -H 'Content-Type: application/json'   -d "{\"username\":\"${SMOKE_USER:-echo}\",\"password\":\"${SMOKE_PASS:-echo123}\"}"   --max-time 15 2>/dev/null || echo "{}")
TOKEN=$(printf '%s' "$LOGIN_RESP" | python -c "
import sys, json
try:
    print(json.load(sys.stdin).get('data', {}).get('accessToken', ''))
except Exception:
    print('')
" 2>/dev/null)
if [ -n "$TOKEN" ] && [ ${#TOKEN} -gt 50 ]; then
  log "[OK  ] 取得 accessToken（长度=${#TOKEN}）"
else
  err "登录取 token 失败（resp=$(printf '%s' "$LOGIN_RESP" | head -c 160)）"
  exit 1
fi

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
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"smoke test"}' \
  --max-time 30 2>/dev/null || echo "000")

if [ "$CREATE_HTTP" = "200" ]; then
  log "[OK  ] create conv HTTP 200"
  # F-182 同族第二处：BFF 响应 id 自 Stage 72 起为**字符串**（"id":"551"），
  # 原正则只匹配裸数字 ⇒ 恒取不到 id ⇒ 契约 5/6 连带跳过。
  CONV_ID=$(grep -oE '"id"[[:space:]]*:[[:space:]]*"?[0-9]+' "$CREATE_RESP" 2>/dev/null | head -1 | grep -oE '[0-9]+')
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
    -H "Authorization: Bearer $TOKEN" \
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
    -H "Authorization: Bearer $TOKEN" \
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
  -H "Authorization: Bearer $TOKEN" \
  --max-time 30 2>/dev/null || echo "000")

if [ "$LISTCONV_HTTP" = "200" ]; then
  log "[OK  ] list convations HTTP 200"
else
  err "list conversations HTTP $LISTCONV_HTTP"
fi

# ---------- 契约 8: gRPC 端点鉴权 (x-user-id metadata) ----------
log "=== 契约 8: 缺身份调用受保护端点应被拦截（网关 jwt-auth，E2E-29 F-182 语义更新）==="
# 不带任何凭据应返 401（jwt-auth 在网关层拦截；此前只测"缺 X-User-Id"是 BFF 层语义）
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

# ---------- 契约 9: SkyWalking OAP rpc.* tag（E2E-26 #12 真断言）----------
# 旧版三重空转已修复，由 scripts/test_smoke_oap_contract.sh 守卫，禁止回退：
#   ① 宿主直连 12800 端口（无宿主映射必超时）→ 改容器网 query_oap.sh
#   ② 伪签名 queryServices 携带 startTimeBucket/endTimeBucket/topN（官方协议不存在，
#      容器网实证 FieldUndefined@[queryServices]）
#   ③ 探测失败只打 WARN 不置 FAIL（OAP 挂 = 静默通过）
log "=== 契约 9: SkyWalking OAP rpc.* tag（真断言，容器网经 query_oap.sh）==="
SMOKE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 9a) OAP 可达 + chat-svc 注册（官方 listServices 签名）
if OAP_SVCS=$(bash "$SMOKE_DIR/query_oap.sh" services 2>&1); then
  if echo "$OAP_SVCS" | grep -q '"emotion-echo-chat-svc"'; then
    log "[OK  ] OAP listServices 含 emotion-echo-chat-svc（gRPC 注册链活）"
  else
    err "契约 9a: OAP 可达但 listServices 无 chat-svc（tracer 未注册）: $OAP_SVCS"
  fi
else
  err "契约 9a: OAP 查询失败（失败必须置 FAIL）: $OAP_SVCS"
fi

# 9b) RPC span 真实落盘：本 smoke 前序 RPC 已产生流量，最近 5 分钟
#     chat-svc 必须有非健康检查类 endpoint 的 trace
if OAP_TRACES=$(bash "$SMOKE_DIR/query_oap.sh" traces emotion-echo-chat-svc 5 2>&1); then
  if echo "$OAP_TRACES" | grep -qE '"traces":\[ *]'; then
    err "契约 9b: 最近 5 分钟 chat-svc 无任何 trace —— server span 未上报 OAP"
  elif echo "$OAP_TRACES" | grep -qE '"endpointNames":\["/(emotion_|api/|chat|auth|users)'; then
    log "[OK  ] OAP 上 chat-svc 有 RPC span（跨进程 trace 已上报）"
  else
    err "契约 9b: 仅有健康检查类 span、无 RPC span（server 端 tracing 断）"
  fi
else
  err "契约 9b: OAP trace 查询失败: $OAP_TRACES"
fi

# ---------- 汇总 ----------
echo
if [ "$FAIL" -gt 0 ]; then
  echo "=== §契约 9 FAIL: $FAIL 项 ==="
  exit 1
fi
echo "=== §契约 9 ALL PASS ==="
exit 0

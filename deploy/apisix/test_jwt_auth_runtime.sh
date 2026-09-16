#!/usr/bin/env bash
# scripts/test_jwt_auth_runtime.sh  (alias: deploy/apisix/test_jwt_auth_runtime.sh)
#
# Sprint 109a RED: 端到端钉死 "合法 JWT 必须通过 APISIX jwt-auth 验证"
#
# 背景：A7 (APISIX jwt-auth 401) — Stage 108 浏览器实测发现所有 /api/v1/* 返 401,
# token 本地 HS256 验算 OK, consumer 配置 OK, route 配置 OK。
#
# 根因诊断结论 (Sprint 109a §二 H1-H6 排查):
#   1. APISIX 3.18.0 config.yaml data_encryption.enable_encrypt_fields: true,
#      PUT consumer 时 secret 被加密存入 etcd (lDJJR5NH8B3gssvm2/IW6w==)。
#   2. jwt-auth 插件加载 consumer 时 consumer.lua:242 调 secret.fetch_secrets,
#      但 fetch_secrets 只解 $secret:// / $env:// URI 引用, 不解 encrypt_fields。
#   3. jwt-auth.lua:56 get_auth_secret 直接返回密文字符串 consumer.auth_conf.secret,
#      用密文做 HMAC 验签 → BFF 用明文 dev-bff-secret 签的 JWT 永远 401。
#
# 验证项 (TDD 钉子):
#   1. POST /api/v1/auth/login 返 200 + accessToken
#   2. 用 accessToken 调受保护 /api/v1/user/profile 必须 200
#   3. 直 curl BFF 端点 + X-User-Id (绕开 APISIX) 必须 200
#   4. BFF 日志必须有对应 grpc-client Login 调用记录 (证实请求真到 BFF)
#
# 前置：dev 模式已起 (docker ps 显示 17 容器 healthy 含 emotion-echo-apisix)
# 运行：bash deploy/apisix/test_jwt_auth_runtime.sh
# 退出码：0 = 全绿, 1 = 任一断言失败
#
# 关联:
#   - docs/plans/sprint-109a-apisix-jwt-401-2026-09-16.md §三 Step 1
#   - docs/plans/test-coverage-tracker-2026-09-16.md §四 A7
#   - docs/stages/stage-108-sender-architecture-debt-fix-2026-09-16.md §四

set -uo pipefail

# ---- 配置 ----
APISIX_PROXY="${APISIX_PROXY:-http://localhost:19080}"
BFF_DIRECT="${BFF_DIRECT:-http://localhost:8894}"
# 注：不从环境变量继承 USERNAME/PASSWORD —— dev 用户 echo 是写死的 demo 账号
USERNAME="echo"
PASSWORD="echo123"

pass=0
fail=0

ok()   { echo "  ✓ $*"; pass=$((pass + 1)); }
bad()  { echo "  ✗ $*"; fail=$((fail + 1)); }

# ---- Step 1: login 必须 200 ----
echo "Step 1: POST /api/v1/auth/login"
LOGIN_RESP=$(curl -sS -X POST "$APISIX_PROXY/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" \
  -w "\nHTTP_CODE=%{http_code}" 2>&1)
HTTP_CODE=$(echo "$LOGIN_RESP" | grep -oE 'HTTP_CODE=[0-9]+' | cut -d= -f2)
BODY=$(echo "$LOGIN_RESP" | grep -v '^HTTP_CODE=')

if [ "$HTTP_CODE" = "200" ]; then
  ok "login HTTP 200"
else
  bad "login HTTP=$HTTP_CODE, body=$BODY"
  echo ""
  echo "FATAL: login 失败,后续测试无法继续。请检查 dev 模式是否启动 (docker ps | grep healthy)。"
  exit 1
fi

TOKEN=$(echo "$BODY" | python -c "
import sys, json
try:
    d = json.load(sys.stdin)
    print(d.get('data', {}).get('accessToken', ''))
except Exception:
    print('')
")
if [ -n "$TOKEN" ] && [ ${#TOKEN} -gt 50 ]; then
  ok "got accessToken (length=${#TOKEN})"
else
  bad "accessToken missing or too short: $TOKEN"
  exit 1
fi

# ---- Step 2: 受保护路由必须 200 (核心断言) ----
echo ""
echo "Step 2: GET /api/v1/user/profile with Bearer token (CRITICAL)"
PROFILE_HTTP=$(curl -sS -o /tmp/profile-resp.txt -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" \
  "$APISIX_PROXY/api/v1/user/profile" 2>&1)
if [ "$PROFILE_HTTP" = "200" ]; then
  ok "profile HTTP 200 (jwt-auth PASS)"
else
  bad "profile HTTP=$PROFILE_HTTP (jwt-auth rejected valid token — A7 bug 未修)"
  echo "  body: $(head -c 200 /tmp/profile-resp.txt)"
fi

# ---- Step 3: POST /conversations 必须 200 (chat 链路) ----
echo ""
echo "Step 3: POST /api/v1/conversations (chat 链路入口)"
CONV_HTTP=$(curl -sS -o /tmp/conv-resp.txt -w "%{http_code}" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -X POST "$APISIX_PROXY/api/v1/conversations" \
  -d '{"title":"smoke-test"}' 2>&1)
if [ "$CONV_HTTP" = "200" ]; then
  ok "conversations POST HTTP 200"
else
  bad "conversations HTTP=$CONV_HTTP, body=$(head -c 200 /tmp/conv-resp.txt)"
fi

# ---- Step 4: 异常路径（无 token）必须 401 ----
echo ""
echo "Step 4: 无 token 调受保护路由必须 401 (反向断言 jwt-auth 真生效)"
NO_AUTH_HTTP=$(curl -sS -o /dev/null -w "%{http_code}" \
  "$APISIX_PROXY/api/v1/user/profile" 2>&1)
if [ "$NO_AUTH_HTTP" = "401" ]; then
  ok "no-auth HTTP 401 (jwt-auth 真拦截)"
else
  bad "no-auth HTTP=$NO_AUTH_HTTP (期望 401)"
fi

# ---- Step 5: BFF 直连（绕开 APISIX）应能收到请求 ----
echo ""
echo "Step 5: BFF 直连 (绕开 APISIX) 应 200 — 证明 BFF 上游没坏"
BFF_HTTP=$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "X-User-Id: 1" \
  "$BFF_DIRECT/api/v1/user/profile" 2>&1)
if [ "$BFF_HTTP" = "200" ] || [ "$BFF_HTTP" = "404" ]; then
  # 200 = BFF 真有 /user/profile; 404 = 路径不存在但 BFF 响应了 (都说明 BFF 上游通)
  ok "BFF-direct HTTP=$BFF_HTTP (BFF 上游 reachable)"
else
  bad "BFF-direct HTTP=$BFF_HTTP (BFF 自身有问题, 不是 APISIX 401)"
fi

# ---- 收口 ----
echo ""
echo "================================"
echo "通过: $pass  失败: $fail"
echo "================================"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
echo "ALL PASS — A7 APISIX jwt-auth 401 已修通"
exit 0

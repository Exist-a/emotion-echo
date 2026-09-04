#!/usr/bin/env bash
# seed_nacos_test.sh：seed.sh 切换 nacos-discovery 后的契约测试
#
# 验证：seed.sh 跑完后，每个 upstream 的 body 形如：
#   { "name": "web-bff", "type": "roundrobin",
#     "discovery_type": "nacos",
#     "service_name": "emotion-echo-web-bff",
#     "discovery_args": { "namespace_id": "emotion-echo-dev",
#                         "group_name": "DEFAULT_GROUP" } }
#
# 而不是静态 nodes。
#
# 2026-09-04 更正：本文件原断言 namespace_id / group_name 是 upstream 的**顶层**字段。
# 那是错的，且从未在真实 APISIX 上跑过——实际 PUT 会被 schema 拒绝：
#   {"error_msg":"invalid configuration: additional properties forbidden, found namespace_id"}
# 按官方文档 https://apisix.apache.org/docs/apisix/discovery/nacos/ ，
# namespace_id / group_name 是 per-service 参数，必须嵌在 upstream.discovery_args 内；
# Nacos 连接信息（host / prefix / fetch_interval / timeout）才属于 config.yaml
# **顶层** discovery.nacos 段（不是 plugin_attr 下）。

set -euo pipefail

ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-WhZEPlrGviCSXlKFfALZlQWinluoGAbj}"
NACOS_NAMESPACE="${NACOS_NAMESPACE:-emotion-echo-dev}"
NACOS_GROUP="${NACOS_GROUP:-DEFAULT_GROUP}"

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }

# 临时文件放脚本同级 .tmp 目录：Git Bash 的 /tmp（含 mktemp -d）是 MSYS 虚拟路径，
# 原生 Windows 的 curl.exe / python.exe 看不到，跨环境会 No such file or directory。
# 用相对路径可让三者都解析到同一位置。
TMPDIR_LOCAL="$(cd "$(dirname "$0")" && pwd)/.tmp-seedtest"
mkdir -p "$TMPDIR_LOCAL"
trap 'rm -rf "$TMPDIR_LOCAL"' EXIT
LOGIN_OUT="$TMPDIR_LOCAL/login.json"
PROT_OUT="$TMPDIR_LOCAL/prot.json"

# 嵌套字段用 grep 取不可靠，统一走 python 解析
json_get() {
  python -c "
import sys, json
try:
    d = json.load(sys.stdin)
except Exception:
    print(''); sys.exit(0)
cur = d
for k in '''$2'''.strip('.').split('.'):
    if not k:
        continue
    cur = cur.get(k) if isinstance(cur, dict) else None
    if cur is None:
        print(''); sys.exit(0)
print(json.dumps(cur) if isinstance(cur, (dict, list)) else cur)
" <<<"$1"
}

echo "=== 契约 1: 6 个 upstream 走 nacos discovery，namespace/group 须嵌在 discovery_args ==="
for id in 1 2 3 4 5 6; do
  body=$(curl -sf -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/upstreams/$id") \
    || fail "upstream $id not found（seed.sh 没跑成功？）"

  discovery_type=$(json_get "$body" '.value.discovery_type')
  [ "$discovery_type" = "nacos" ] \
    || fail "upstream $id discovery_type='$discovery_type'（期望 'nacos'）"

  service_name=$(json_get "$body" '.value.service_name')
  case "$service_name" in
    emotion-echo-*) : ;;
    *) fail "upstream $id service_name='$service_name'（期望 emotion-echo-* 长名，PR-0 约定）" ;;
  esac

  # 关键回归断言：必须在 discovery_args 内，且顶层不得出现
  ns=$(json_get "$body" '.value.discovery_args.namespace_id')
  gp=$(json_get "$body" '.value.discovery_args.group_name')
  [ "$ns" = "$NACOS_NAMESPACE" ] \
    || fail "upstream $id discovery_args.namespace_id='$ns'（期望 '$NACOS_NAMESPACE'）"
  [ "$gp" = "$NACOS_GROUP" ] \
    || fail "upstream $id discovery_args.group_name='$gp'（期望 '$NACOS_GROUP'）"

  top_ns=$(json_get "$body" '.value.namespace_id')
  [ -z "$top_ns" ] \
    || fail "upstream $id 把 namespace_id 放在顶层了——APISIX schema 会拒绝该形状"

  nodes=$(json_get "$body" '.value.nodes')
  [ -z "$nodes" ] \
    || fail "upstream $id 仍带静态 nodes=$nodes（应完全交给 nacos discovery）"

  pass "upstream $id ($service_name) nacos discovery + discovery_args 正确"
done

echo
echo "=== 契约 2: 目标服务在 Nacos 注册表里真有实例（否则路由必 503） ==="
for id in 1 2 3 4 5 6; do
  body=$(curl -sf -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/upstreams/$id")
  service_name=$(json_get "$body" '.value.service_name')
  hosts=$(curl -sf "http://localhost:8848/nacos/v1/ns/instance/list?serviceName=${service_name}&namespaceId=${NACOS_NAMESPACE}&groupName=${NACOS_GROUP}" 2>/dev/null \
          | grep -oE '"ip":"[0-9.]+"' | wc -l | tr -d ' ')
  [ "${hosts:-0}" -ge 1 ] \
    || fail "$service_name 在 Nacos 注册表 0 实例——APISIX 无法解析上游"
  pass "$service_name 在 Nacos 有 $hosts 个实例"
done

echo
echo "=== 契约 3: 端到端——经 APISIX 登录必须成功（证明 discovery 真生效） ==="
GW_URL="${APISIX_GATEWAY_URL:-http://localhost:19080}"
code=$(curl -s -o "$LOGIN_OUT" -w '%{http_code}' \
  -X POST "$GW_URL/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d '{"username":"echo","password":"echo123"}')
[ "$code" = "200" ] \
  || fail "经 APISIX 登录返 HTTP $code（body: $(head -c 200 "$LOGIN_OUT")）"
grep -q '"accessToken"' "$LOGIN_OUT" \
  || fail "经 APISIX 登录 200 但无 accessToken：$(head -c 200 "$LOGIN_OUT")"
pass "经 APISIX 登录成功并返回 accessToken"

TOKEN=$(python -c "import sys,json;print(json.load(sys.stdin)['data']['accessToken'])" <"$LOGIN_OUT")

echo
echo "=== 契约 4: 受保护端点经 APISIX 必须 200（jwt-auth consumer + X-User-Id 注入都到位） ==="
# 这条抓两类历史断链：
#   1. seed.sh 从没建 jwt-auth consumer → 所有受保护路由必 401
#   2. seed.sh 从没真正注入 X-User-Id（只在 cors 里列了名单）→ BFF 必 401
for p in "users/me" "conversations" "surveys"; do
  code=$(curl -s -o "$PROT_OUT" -w '%{http_code}' \
    "$GW_URL/api/v1/$p" -H "Authorization: Bearer $TOKEN")
  [ "$code" = "200" ] \
    || fail "/$p 经 APISIX 返 HTTP $code（body: $(head -c 200 "$PROT_OUT")）"
  pass "/$p 经 APISIX 200"
done

echo
echo "=== 契约 5: X-User-Id 不可被客户端伪造 ==="
# 注入必须是覆盖式：客户端塞 X-User-Id: 999 也只能拿到自己的 user_id
curl -s "$GW_URL/api/v1/users/me" \
  -H "Authorization: Bearer $TOKEN" -H "X-User-Id: 999" >"$PROT_OUT"
real_uid=$(python -c "
import json,sys
try: print(json.load(sys.stdin)['data']['user']['userId'])
except Exception: print('parse-error')
" <"$PROT_OUT")
[ "$real_uid" != "999" ] \
  || fail "伪造 X-User-Id:999 被下游采信——proxy 注入不是覆盖式，存在越权风险"
[ "$real_uid" != "parse-error" ] \
  || fail "无法解析 users/me 响应：$(head -c 200 "$PROT_OUT")"
pass "伪造 X-User-Id 被覆盖（实际 userId=$real_uid）"

echo
echo "=== 契约 6: 未鉴权请求必须 401 ==="
code=$(curl -s -o /dev/null -w '%{http_code}' "$GW_URL/api/v1/users/me")
[ "$code" = "401" ] || fail "无 token 访问受保护端点返 HTTP $code（期望 401）"
pass "无 token → 401"

code=$(curl -s -o /dev/null -w '%{http_code}' "$GW_URL/api/v1/users/me" -H "X-User-Id: 999")
[ "$code" = "401" ] || fail "仅伪造 X-User-Id 无 token 返 HTTP $code（期望 401）"
pass "仅伪造 X-User-Id 无 token → 401"

echo
echo "seed nacos 契约全部 PASS"

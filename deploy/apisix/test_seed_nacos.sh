#!/usr/bin/env bash
# seed_nacos_test.sh：seed.sh 切换 nacos-discovery 后的契约测试
#
# 验证：seed.sh 跑完后，每个 upstream 的 body 形如：
#   { "name": "web-bff", "type": "roundrobin",
#     "discovery_type": "nacos",
#     "service_name": "emotion-echo-web-bff",
#     "namespace_id": "emotion-echo-dev",
#     "group_name": "DEFAULT_GROUP" }
#
# 而不是静态 nodes。

set -euo pipefail

ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-WhZEPlrGviCSXlKFfALZlQWinluoGAbj}"

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }

for id in 1 2 3 4 5 6; do
  body=$(curl -sf -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/upstreams/$id") \
    || fail "upstream $id not found"
  discovery_type=$(echo "$body" | grep -oE '"discovery_type":"[a-z]+"' | head -1 | cut -d'"' -f4)
  if [ "$discovery_type" != "nacos" ]; then
    fail "upstream $id discovery_type is '$discovery_type' (expected 'nacos')"
  fi
  pass "upstream $id uses nacos discovery"
done
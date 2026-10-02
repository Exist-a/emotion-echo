#!/usr/bin/env bash
# E2E-26 测试点 #12 · smoke_bff_chat_grpc.sh 契约 9 空转修复守卫（先红后绿）
#
# 背景（E2E-26 计划期 F-e + 执行期容器网实证）：
#   原契约 9 三重空转——
#   ① 宿主 `curl localhost:12800/graphql`（端口无映射 ⇒ 必超时）
#   ② 查询签名 queryServices(serviceId:,startTimeBucket:,endTimeBucket:,topN:)
#      在官方 metadata v1/v2 / metrics v2/v3 / topology 五个协议文件中均不存在
#      （容器网实发实证：Validation error FieldUndefined@queryServices）
#   ③ 两层失败都落 [WARN] best-effort 不计入 FAIL ⇒ 契约 9 从未真正通过
#
# 本守卫钉死修复后的四条：
#   O1 不得宿主直连 localhost:12800
#   O2 不得保留协议不存在的 queryServices 签名
#   O3 契约 9 不得 best-effort 吞错（OAP 挂 = 真 FAIL）
#   O4 必须改走 scripts/query_oap.sh（容器网 + 官方 Duration 格式）
#
# 负向对照：把契约 9 改回任一旧形态 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SMOKE="$REPO_ROOT/scripts/smoke_bff_chat_grpc.sh"
FAIL=0

pass() { printf '[OK  ] %s\n' "$*"; }
fail() { printf '[FAIL] %s\n' "$*"; FAIL=$((FAIL + 1)); }

if [ ! -f "$SMOKE" ]; then
  echo "[FAIL] smoke_bff_chat_grpc.sh 不存在"
  exit 1
fi
if ! bash -n "$SMOKE" 2>/dev/null; then
  fail "smoke_bff_chat_grpc.sh bash -n 语法失败"
fi

# O1: 禁宿主直连
if grep -q 'localhost:12800' "$SMOKE"; then
  fail "O1 仍直连 localhost:12800（宿主端口无映射必超时，F-c）"
else
  pass "O1 无宿主 12800 直连"
fi

# O2: 禁协议不存在的签名
if grep -q 'queryServices(serviceId' "$SMOKE"; then
  fail "O2 仍使用协议不存在的 queryServices(serviceId:,startTimeBucket:...) 签名（容器网实证 FieldUndefined）"
else
  pass "O2 无伪签名"
fi

# O3: 禁 best-effort 吞错（OAP 挂 = 真 FAIL）
if grep -q 'best-effort' "$SMOKE" || grep -q '不计入 FAIL' "$SMOKE"; then
  fail "O3 契约 9 仍含 best-effort/不计入 FAIL 吞错语义——OAP 挂必须真 FAIL（负向对照：探测失败置 FAIL）"
else
  pass "O3 无 best-effort 吞错"
fi

# O4: 必须复用 query_oap.sh（容器网 + 合规格式的唯一入口）
if grep -q 'query_oap.sh' "$SMOKE"; then
  pass "O4 走 query_oap.sh"
else
  fail "O4 必须调用 scripts/query_oap.sh（否则自行拼查询会重蹈格式/视角两类错）"
fi

# O5: OAP 失败路径必须落到 err（FAIL 计数），不能只 log WARN
if grep -qE 'err .*OAP|err.*契约 9' "$SMOKE"; then
  pass "O5 OAP 失败走 err（计入 FAIL）"
else
  fail "O5 契约 9 的 OAP 失败必须调用 err（计入 FAIL 退出码）"
fi

printf '\n'
if [ "$FAIL" -gt 0 ]; then
  printf '=== test_smoke_oap_contract: FAIL %d ===\n' "$FAIL"
  exit 1
fi
echo "=== test_smoke_oap_contract: ALL PASS ==="

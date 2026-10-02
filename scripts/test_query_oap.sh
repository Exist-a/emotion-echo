#!/usr/bin/env bash
# E2E-26 测试点 #8 · OAP 查询工具契约测试（先红后绿）
#
# 背景（E2E-26 计划期/执行期实测 F-c/F-e）：
#   ① 宿主 localhost:12800 无端口映射（Stage 33 端口收紧）⇒ 宿主 curl 恒超时；
#   ② smoke_bff_chat_grpc.sh 契约 9 用宿主 curl + 不存在的 GraphQL 签名 +
#      best-effort WARN 不计 FAIL ⇒ 从未真验过；
#   ③ 历史「OAP 9.x queryDuration bug」归因存疑——官方 common.graphqls 明确
#      SECOND 步长格式 = yyyy-MM-dd HHmmss（**无冒号**），带冒号输入即报
#      "malformed" 类校验错误。
#
# 本守卫钉死 scripts/query_oap.sh 的四条契约：
#   C1 官方 Duration 格式（SECOND 无冒号）
#   C2 走容器网访问（禁止宿主 localhost:12800 直连）
#   C3 dry-run 可见合规查询（step: SECOND + queryBasicTraces）
#   C4 失败即非零退出（禁止 best-effort 静默成功）
#
# 负向对照：把格式改回带冒号 / 改回宿主直连 / 加 best-effort 吞错 → 本脚本非零退出。

set -uo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOL="$REPO_ROOT/scripts/query_oap.sh"
FAIL=0

note() { printf '%s\n' "$*"; }
fail() { printf '[FAIL] %s\n' "$*"; FAIL=$((FAIL + 1)); }
pass() { printf '[OK  ] %s\n' "$*"; }

# ---- C0: 工具存在且是 bash 脚本 ----
if [ ! -f "$TOOL" ]; then
  fail "C0 scripts/query_oap.sh 不存在（E2E-26 #8 查询工具未落地）"
  printf '=== FAIL: %d ===\n' "$FAIL"
  exit 1
fi
pass "C0 查询工具存在"

# ---- C1: 官方 Duration 格式（SECOND 步长 yyyy-MM-dd HHmmss，无冒号） ----
FMT_OUT=$(bash "$TOOL" format-duration 1790954775 2>&1)
if printf '%s' "$FMT_OUT" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{6}$'; then
  pass "C1 format-duration 输出合规: $FMT_OUT"
else
  fail "C1 format-duration 输出不合规（期望 yyyy-MM-dd HHmmss 无冒号，官方 query-protocol common.graphqls）: got '$FMT_OUT'"
fi
if printf '%s' "$FMT_OUT" | grep -q ':'; then
  fail "C1 输出含冒号 —— OAP SECOND 步长不接受带冒号格式（历史 queryDuration 报错的真因）"
fi

# ---- C2: 容器网访问，禁止宿主 localhost:12800 直连 ----
if grep -q 'localhost:12800' "$TOOL"; then
  fail "C2 工具直连 localhost:12800 —— 宿主端口未映射（F-c）必超时"
else
  pass "C2 无宿主 12800 直连"
fi
if grep -q -- '--network' "$TOOL" && grep -q '12800/graphql' "$TOOL"; then
  pass "C2 走 docker network 访问容器内 OAP"
else
  fail "C2 必须经 docker network 访问 skywalking-oap:12800（宿主无端口映射）"
fi

# ---- C3: dry-run 输出合规查询（不发请求也能审计查询构造） ----
DRY_OUT=$(bash "$TOOL" dry-run traces svc-abc 2>&1)
DRY_RC=$?
if [ "$DRY_RC" -ne 0 ]; then
  fail "C3 dry-run 非零退出（rc=$DRY_RC）: $DRY_OUT"
else
  if printf '%s' "$DRY_OUT" | grep -q 'queryBasicTraces' \
     && printf '%s' "$DRY_OUT" | grep -q 'step: SECOND'; then
    pass "C3 dry-run 查询含 queryBasicTraces + step: SECOND"
  else
    fail "C3 dry-run 查询构造不合规: $DRY_OUT"
  fi
  # 生成的 start/end 必须是无冒号 SECOND 格式（从 dry-run 输出提取）
  BAD_T=$(printf '%s' "$DRY_OUT" | grep -oE '"[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}"' | head -1)
  if [ -n "$BAD_T" ]; then
    fail "C3 查询里出现带冒号时间串: $BAD_T"
  else
    pass "C3 查询无带冒号时间串"
  fi
fi

# ---- C4: 失败即非零退出（禁止 best-effort 静默） ----
if grep -qi 'best-effort' "$TOOL"; then
  fail "C4 工具含 best-effort 语义 —— 失败不得静默成功（契约 9 空转的病根）"
else
  pass "C4 无 best-effort 吞错"
fi
if grep -q '"errors"' "$TOOL"; then
  pass "C4 检测 GraphQL errors 响应"
else
  fail "C4 必须检测响应中的 errors 字段并置非零退出"
fi

printf '\n'
if [ "$FAIL" -gt 0 ]; then
  printf '=== test_query_oap: FAIL %d ===\n' "$FAIL"
  exit 1
fi
echo "=== test_query_oap: ALL PASS ==="

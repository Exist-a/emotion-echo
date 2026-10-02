#!/usr/bin/env bash
# E2E-25 C2：APISIX seed↔admin 漂移检测工具（F-139 治理，#7/#8/#10）
#
# 背景：seed.sh 每次全量 PUT 会**静默覆盖** admin 手工改动（篡改类漂移），
# 且不清理白名单外的路由（Stage 112 route 116 / E2E-25 实测 route 299，额外类漂移）。
# 本工具把"运行时配置 vs seed 定义"变成可机检的 diff。纯逻辑在
# scripts/apisix_drift_lib.js（有契约测试），本脚本只做 curl 编排。
#
# 用法：
#   check_apisix_drift.sh snapshot <outfile>     # 导出运行时规范化快照（需网关）
#   check_apisix_drift.sh diff <before> <after>  # 对比两份快照；有差异 exit 2
#   check_apisix_drift.sh extras-from <routes.json>  # 离线：JSON 路由列表 vs seed 白名单
#   check_apisix_drift.sh extras                 # 运行时：白名单外路由；有则 exit 1（需网关）
#   check_apisix_drift.sh verify                 # 快照→重跑 seed→快照→diff+extras（需网关）
#
# 退出码：0 干净；1 extras（白名单外对象存活）；2 篡改类漂移（seed 已把配置拉回
#          白名单，需人工确认改动来源——处置策略 M2 待裁定前先报不拦）。
#
# env：APISIX_ADMIN_URL（默认 http://localhost:9180）/ APISIX_ADMIN_KEY（同 seed.sh）/
#      SEED_SH（默认 repo 内 deploy/apisix/seed.sh）/ NODE_BIN（默认 node）
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
LIB="$SCRIPT_DIR/apisix_drift_lib.js"
ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-dev-admin-key-local-only}"
SEED_SH="${SEED_SH:-$SCRIPT_DIR/../deploy/apisix/seed.sh}"
NODE_BIN="${NODE_BIN:-node}"

log() { echo "[apisix-drift] $*" >&2; }
die() { echo "[apisix-drift] FATAL: $*" >&2; exit "${2:-1}"; }

[ -f "$LIB" ] || die "lib not found: $LIB"

fetch_raw() {
  local r u c
  r=$(curl -sf -m 10 -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/routes") \
    || die "admin routes fetch failed at $ADMIN_URL"
  u=$(curl -sf -m 10 -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/upstreams") \
    || die "admin upstreams fetch failed"
  c=$(curl -sf -m 10 -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/consumers") \
    || die "admin consumers fetch failed"
  printf '{"routes":%s,"upstreams":%s,"consumers":%s}' "$r" "$u" "$c"
}

MODE="${1:-}"
case "$MODE" in
  snapshot)
    OUT="${2:?usage: snapshot <outfile>}"
    fetch_raw | "$NODE_BIN" "$LIB" normalize > "$OUT"
    log "snapshot written: $OUT ($(wc -l < "$OUT") lines)"
    ;;

  diff)
    BEFORE="${2:?usage: diff <before> <after>}"
    AFTER="${3:?usage: diff <before> <after>}"
    RC=0
    "$NODE_BIN" "$LIB" diff "$BEFORE" "$AFTER" || RC=$?
    exit "$RC"
    ;;

  extras-from)
    FILE="${2:?usage: extras-from <routes.json>}"
    RC=0
    "$NODE_BIN" "$LIB" extras-from "$SEED_SH" "$FILE" || RC=$?
    exit "$RC"
    ;;

  extras)
    TMP=$(mktemp)
    trap 'rm -f "$TMP"' EXIT
    curl -sf -m 10 -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL/apisix/admin/routes" > "$TMP" \
      || die "admin routes fetch failed"
    RC=0
    "$NODE_BIN" "$LIB" extras-from "$SEED_SH" "$TMP" || RC=$?
    if [ "$RC" -ne 0 ]; then
      log "FAIL: routes outside seed whitelist detected (route 116/299 类漂移)"
      exit 1
    fi
    log "no extras"
    ;;

  verify)
    TMP=$(mktemp -d)
    trap 'rm -rf "$TMP"' EXIT
    log "step 1/3: snapshot before"
    "$0" snapshot "$TMP/before.txt"
    log "step 2/3: re-running seed ($SEED_SH)"
    SKIP_HEALTH_CHECK="${SKIP_HEALTH_CHECK:-true}" bash "$SEED_SH"
    log "step 3/3: snapshot after + compare"
    "$0" snapshot "$TMP/after.txt"
    RC=0
    if ! "$0" diff "$TMP/before.txt" "$TMP/after.txt"; then
      log "NOTE: 上述 diff = seed 覆盖/清理了运行时漂移（篡改类）。处置策略 M2 待裁定，本轮只报不拦"
    fi
    if ! "$0" extras; then
      RC=1
    fi
    [ "$RC" -eq 0 ] && log "verify clean"
    exit "$RC"
    ;;

  *)
    die "usage: check_apisix_drift.sh {snapshot|diff|extras-from|extras|verify} ..." 1
    ;;
esac

#!/usr/bin/env bash
# scripts/rotate_jwt_secret.sh — JWT 密钥轮换编排/校验（E2E-29 D-48 / ADR-2026-10-jwt-key-rotation-dual-key）
#
# 本脚本**只做编排与校验，不读写密钥文件**：AGENTS §四红线规定密钥只进 gitignored 的
# deploy/.env.local（prod 走密钥管理系统），脚本不代写、不回显密钥内容。
#
# 子命令：
#   --status              查看当前窗口状态（网关侧 consumer 有几条、各自 key id）
#   --plan <新密钥>       打印轮换四步（含需要写入密钥存放处的变量），不做任何改动
#   --verify              窗口期校验：新旧两条 consumer 是否都在（并给出用哪把密钥签的 token 会被谁验）
#   --finalize            收尾提示：清掉 _PREV 后重跑 seed（seed 会自动删除 prev consumer）
#
# 退出码：0 正常；2 参数/环境问题；3 网关不可达

set -uo pipefail

ADMIN_URL="${APISIX_ADMIN_URL:-http://localhost:9180}"
ADMIN_KEY="${APISIX_ADMIN_KEY:-}"
CONSUMER="emotion_echo_bff"
CONSUMER_PREV="emotion_echo_bff_prev"

log() { echo "[rotate] $*"; }
err() { echo "[rotate] ERROR: $*" >&2; }

admin_get() { # $1=path
  curl -sf -H "X-API-KEY: $ADMIN_KEY" "$ADMIN_URL$1" 2>/dev/null
}

require_admin() {
  if [ -z "$ADMIN_KEY" ]; then
    # 与 seed.sh 同源：HOST 侧从 deploy/.env.local 自动取（不打印内容）
    env_file="$(cd "$(dirname "$0")/.." && pwd)/deploy/.env.local"
    if [ -f "$env_file" ]; then
      # shellcheck disable=SC1090
      ADMIN_KEY="$(grep -E '^APISIX_ADMIN_KEY=' "$env_file" | head -1 | cut -d= -f2- | tr -d '"'"'"'')"
    fi
  fi
  if [ -z "$ADMIN_KEY" ]; then
    err "APISIX_ADMIN_KEY 未提供（可 export，或确保 deploy/.env.local 里有）"
    exit 2
  fi
  if ! admin_get "/apisix/admin/consumers/$CONSUMER" >/dev/null; then
    err "网关 admin API 不可达或密钥不对：$ADMIN_URL"
    exit 3
  fi
}

consumer_key() { # $1=consumer id
  # 注意：必须按 **bytes 读 + 显式 UTF-8 解码**。Windows 下 python 从 stdin 默认按本地
  # 码页解码，而 consumer 的 desc 含中文 ⇒ UnicodeDecodeError 被 except 吞掉 ⇒ 静默输出空
  # （实测踩过：--status 打印 "key=" 却仍报成功）。
  admin_get "/apisix/admin/consumers/$1" | python -c "
import sys, json
raw = sys.stdin.buffer.read()
if not raw:
    print('')
    sys.exit(0)
try:
    d = json.loads(raw.decode('utf-8', errors='replace'))
    v = d.get('value', d)
    print(v.get('plugins', {}).get('jwt-auth', {}).get('key', ''))
except Exception:
    print('')
"
}

case "${1:---status}" in
  --status|--verify)
    require_admin
    CUR_KEY="$(consumer_key "$CONSUMER")"
    PREV_KEY="$(consumer_key "$CONSUMER_PREV")"
    if [ -z "$CUR_KEY" ]; then
      err "读不到 $CONSUMER 的 jwt-auth key（网关响应异常或 consumer 不存在）——不视为正常"
      exit 3
    fi
    log "当前 consumer  : $CONSUMER      key=$CUR_KEY"
    if [ -n "$PREV_KEY" ]; then
      log "上一把 consumer: $CONSUMER_PREV key=$PREV_KEY  ⇒ **窗口开着**"
      log "收尾：清掉 BFF_JWT_KEY_ID_PREV / BFF_JWT_SECRET_PREV 后重跑 bash deploy/apisix/seed.sh"
    else
      log "上一把 consumer: （不存在）⇒ 无窗口，单密钥状态"
    fi
    if [ "$1" = "--verify" ] && [ -z "$PREV_KEY" ]; then
      err "窗口期校验失败：prev consumer 不存在（窗口未开）"
      exit 2
    fi
    ;;

  --plan)
    NEW_SECRET="${2:-}"
    if [ -z "$NEW_SECRET" ]; then
      err "用法：bash scripts/rotate_jwt_secret.sh --plan <新密钥>"
      exit 2
    fi
    cat <<PLAN
[rotate] 轮换四步（详见 docs/architecture/adr/adr-2026-10-jwt-key-rotation-dual-key.md）
  1) 在密钥存放处（dev = deploy/.env.local；prod = 密钥管理系统）改为：
       BFF_JWT_KEY_ID_PREV=<当前 keyID，通常是 user>
       BFF_JWT_SECRET_PREV=<当前密钥>
       BFF_JWT_KEY_ID=<新 keyID，例如 user-v2>
       BFF_JWT_SECRET=<新密钥，见下>
     —— 新密钥（仅打印一次，请立即存入密钥存放处，不要贴进任何会提交的文件）：
       $NEW_SECRET
  2) bash deploy/apisix/seed.sh          # 期望出现两条 consumer
  3) 重建/重启 BFF，然后 bash scripts/rotate_jwt_secret.sh --verify
     （要求：旧 token 与新建 token 都能经网关 200）
  4) 等 ≥ 一个 token TTL 后：清掉两个 _PREV 变量并重跑 seed（prev consumer 会被自动删除）
PLAN
    ;;

  --finalize)
    cat <<'FIN'
[rotate] 收尾（步 4）：
  1) 从密钥存放处删除 BFF_JWT_KEY_ID_PREV 与 BFF_JWT_SECRET_PREV
  2) bash deploy/apisix/seed.sh     # 未配置 _PREV 时脚本会删除 emotion_echo_bff_prev
  3) bash scripts/rotate_jwt_secret.sh --status   # 期望：上一把 consumer 不存在
FIN
    ;;

  *)
    err "未知参数：$1（支持 --status / --plan <新密钥> / --verify / --finalize）"
    exit 2
    ;;
esac

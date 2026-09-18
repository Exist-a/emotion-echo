#!/usr/bin/env bash
# cleanup-demo-account.sh — 清理演示账号（E2E-06）
#
# 用途：删除演示账号及其关联数据（密保问题、聊天记录等）
#
# 环境变量：
#   DEMO_USERNAME — 演示账号用户名（默认 echo）
#   PGHOST        — Postgres 主机（默认 localhost）
#   PGPORT        — Postgres 端口（默认 5432）
#   PGUSER        — Postgres 用户（默认 postgres）
#   PGPASSWORD    — Postgres 密码（默认 postgres）
#   PGDATABASE    — 数据库名（默认 emotion_echo）
#
# 输出：删除的记录数

set -euo pipefail

DEMO_USERNAME="${DEMO_USERNAME:-echo}"
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"
PGPASSWORD="${PGPASSWORD:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"
export PGPASSWORD

log() { echo "[cleanup-demo] $*" >&2; }
die() { echo "[cleanup-demo] FATAL: $*" >&2; exit 1; }

# 检查依赖
command -v psql >/dev/null 2>&1 || die "psql not found"

# 获取用户 ID
USER_ID=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "SELECT id FROM emotion_echo_user.users WHERE username = '$DEMO_USERNAME';" 2>/dev/null | tr -d ' ')

if [ -z "$USER_ID" ] || [ "$USER_ID" = "" ]; then
    log "Demo user '$DEMO_USERNAME' not found, nothing to clean up"
    exit 0
fi

log "Found demo user: $DEMO_USERNAME (ID: $USER_ID)"

# 删除关联数据（按外键依赖顺序）
log "Cleaning up related data..."

# 1. 密保问题
SEC_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_user.user_security_answers WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Security answers deleted: $SEC_DELETED"

# 2. 刷新令牌
TOKEN_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_user.refresh_tokens WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Refresh tokens deleted: $TOKEN_DELETED"

# 3. 上传文件记录
FILE_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_user.upload_files WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Upload files deleted: $FILE_DELETED"

# 4. 聊天消息（通过会话级联）
MSG_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_chat.messages WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Chat messages deleted: $MSG_DELETED"

# 5. 聊天会话
CONV_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_chat.conversations WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Conversations deleted: $CONV_DELETED"

# 6. 用户行为事件
EVENT_DELETED=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "DELETE FROM emotion_echo_analytics.user_behavior_events WHERE user_id = $USER_ID;" 2>/dev/null | grep -o '[0-9]*' || echo "0")
log "  Behavior events deleted: $EVENT_DELETED"

# 7. 用户本身
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 -c \
    "DELETE FROM emotion_echo_user.users WHERE id = $USER_ID;" >/dev/null 2>&1

log "✅ Demo user '$DEMO_USERNAME' (ID: $USER_ID) cleaned up"
log ""
log "   To recreate: bash deploy/db/seed-demo-account.sh"
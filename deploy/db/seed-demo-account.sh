#!/usr/bin/env bash
# seed-demo-account.sh — 可重跑的演示账号种子脚本（E2E-06）
#
# 用途：创建/更新演示账号，自带密保问题（供 E2E-07 找回密码演示）
#
# 环境变量：
#   DEMO_USERNAME    — 演示账号用户名（默认 echo）
#   DEMO_PASSWORD    — 演示账号密码（默认 echo123）
#   DEMO_NICKNAME    — 演示账号昵称（默认 Echo User）
#   PGHOST           — Postgres 主机（默认 localhost）
#   PGPORT           — Postgres 端口（默认 5432）
#   PGUSER           — Postgres 用户（默认 postgres）
#   PGPASSWORD       — Postgres 密码（默认 postgres）
#   PGDATABASE       — 数据库名（默认 emotion_echo）
#
# 幂等性：ON CONFLICT (username) DO UPDATE，可重复执行
# 输出：创建/更新的用户信息 + 密保问题设置结果

set -euo pipefail

DEMO_USERNAME="${DEMO_USERNAME:-echo}"
DEMO_PASSWORD="${DEMO_PASSWORD:-echo123}"
DEMO_NICKNAME="${DEMO_NICKNAME:-Echo User}"
PGHOST="${PGHOST:-localhost}"
PGPORT="${PGPORT:-5432}"
PGUSER="${PGUSER:-postgres}"
PGPASSWORD="${PGPASSWORD:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"
export PGPASSWORD

log() { echo "[seed-demo] $*" >&2; }
die() { echo "[seed-demo] FATAL: $*" >&2; exit 1; }

# 检查依赖
command -v psql >/dev/null 2>&1 || die "psql not found"

# 计算 bcrypt hash（需要 python + bcrypt 库）
compute_hash() {
    local password="$1"
    if command -v python3 >/dev/null 2>&1; then
        python3 -c "import bcrypt; print(bcrypt.hashpw(b'$password', bcrypt.gensalt(10)).decode())"
    elif command -v python >/dev/null 2>&1; then
        python -c "import bcrypt; print(bcrypt.hashpw(b'$password', bcrypt.gensalt(10)).decode())"
    else
        # 兜底：使用预计算的 hash（echo123）
        if [ "$password" = "echo123" ]; then
            echo '$2a$10$x/oarv7WP0HJBNTiJGJBSeBMCvqIS.jMndnYasMS.O2SLzm7pqQnC'
        else
            die "python + bcrypt required for custom password"
        fi
    fi
}

# 计算密码 hash
log "Computing bcrypt hash for demo user..."
PASSWORD_HASH=$(compute_hash "$DEMO_PASSWORD")

# 创建/更新演示账号
log "Creating/updating demo user: $DEMO_USERNAME"
psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 <<EOF
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, created_at, updated_at
) VALUES (
    '$DEMO_USERNAME',
    '$PASSWORD_HASH',
    '$DEMO_NICKNAME',
    NOW(),
    NOW()
)
ON CONFLICT (username) DO UPDATE SET
    password_hash = EXCLUDED.password_hash,
    nickname = EXCLUDED.nickname,
    updated_at = NOW();
EOF

# 获取用户 ID
USER_ID=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "SELECT id FROM emotion_echo_user.users WHERE username = '$DEMO_USERNAME';" | tr -d ' ')
log "Demo user ID: $USER_ID"

# 设置密保问题（1~2 个）
log "Setting security questions for demo user..."

# 密保问题 1
SECURITY_Q1="你的第一只宠物叫什么名字？"
SECURITY_A1="小花"
SECURITY_HASH1=$(compute_hash "$SECURITY_A1")

# 密保问题 2
SECURITY_Q2="你出生在哪个城市？"
SECURITY_A2="北京"
SECURITY_HASH2=$(compute_hash "$SECURITY_A2")

psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -v ON_ERROR_STOP=1 <<EOF
-- 密保问题 1
INSERT INTO emotion_echo_user.user_security_answers (
    user_id, question_order, question, answer_hash, created_at
) VALUES (
    $USER_ID, 1, '$SECURITY_Q1', '$SECURITY_HASH1', NOW()
)
ON CONFLICT (user_id, question_order) DO UPDATE SET
    question = EXCLUDED.question,
    answer_hash = EXCLUDED.answer_hash;

-- 密保问题 2
INSERT INTO emotion_echo_user.user_security_answers (
    user_id, question_order, question, answer_hash, created_at
) VALUES (
    $USER_ID, 2, '$SECURITY_Q2', '$SECURITY_HASH2', NOW()
)
ON CONFLICT (user_id, question_order) DO UPDATE SET
    question = EXCLUDED.question,
    answer_hash = EXCLUDED.answer_hash;
EOF

# 验证
log "Verifying demo account..."
USER_COUNT=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "SELECT COUNT(*) FROM emotion_echo_user.users WHERE username = '$DEMO_USERNAME';" | tr -d ' ')
SEC_COUNT=$(psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -d "$PGDATABASE" -t -c \
    "SELECT COUNT(*) FROM emotion_echo_user.user_security_answers WHERE user_id = $USER_ID;" | tr -d ' ')

if [ "$USER_COUNT" != "1" ]; then
    die "Verification failed: user count = $USER_COUNT"
fi
if [ "$SEC_COUNT" != "2" ]; then
    die "Verification failed: security answers count = $SEC_COUNT"
fi

log "✅ Demo account ready:"
log "   Username: $DEMO_USERNAME"
log "   Password: $DEMO_PASSWORD"
log "   Nickname: $DEMO_NICKNAME"
log "   Security Q1: $SECURITY_Q1 → $SECURITY_A1"
log "   Security Q2: $SECURITY_Q2 → $SECURITY_A2"
log ""
log "   To clean up: bash deploy/db/cleanup-demo-account.sh"
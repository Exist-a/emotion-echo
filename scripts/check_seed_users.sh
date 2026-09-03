#!/usr/bin/env bash
# Stage 38-A: 默认测试账号 smoke 验证（echo / echo123）
#
# 演进：
#   Stage 36-D：账号 = 13800138000 / abc123
#   Stage 38-A：账号 = echo / echo123（纯 dev 演示用户名+密码）
#
# 用法：
#   ./scripts/check_seed_users.sh                  # RED: 期望 exit 1
#   ./scripts/check_seed_users.sh && echo ok       # GREEN: 期望 ok
#
# 检查项：
#   1) DB 里 emotion_echo_user.users 表存在 username='echo' 的行
#   2) BFF POST /api/v1/auth/login {username:'echo',password:'echo123'} 返回 200
set -uo pipefail

CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
DB="${PG_DB:-emotion_echo}"
USER="${PG_USER:-postgres}"
EXPECTED_USERNAME="${EXPECTED_USERNAME:-echo}"
EXPECTED_PASSWORD="${EXPECTED_PASSWORD:-echo123}"
BFF="${BFF:-http://localhost:8894}"

FAIL=0

# Step 1: 检查 DB
COUNT=$(MSYS_NO_PATHCONV=1 docker exec "$CONTAINER" \
    psql -U "$USER" -d "$DB" -tA \
    -c "SELECT COUNT(*) FROM emotion_echo_user.users WHERE username='$EXPECTED_USERNAME'" 2>/dev/null || echo "ERR")

if [[ "$COUNT" == "1" ]]; then
    echo "[OK  ] DB has user '$EXPECTED_USERNAME'"
else
    echo "[FAIL] DB missing user '$EXPECTED_USERNAME' (count=$COUNT)"
    FAIL=1
fi

# Step 2: 检查 BFF 登录
if command -v curl >/dev/null 2>&1; then
    LOGIN_HTTP=$(curl -sS -o /tmp/login_resp.json -w '%{http_code}' \
        -X POST "$BFF/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d "{\"username\":\"$EXPECTED_USERNAME\",\"password\":\"$EXPECTED_PASSWORD\"}" \
        --max-time 10 2>/dev/null || echo "000")

    if [[ "$LOGIN_HTTP" == "200" ]]; then
        echo "[OK  ] BFF /api/v1/auth/login returns 200"
    else
        echo "[FAIL] BFF /api/v1/auth/login returns $LOGIN_HTTP (resp=$(head -c 120 /tmp/login_resp.json))"
        FAIL=1
    fi
else
    echo "[SKIP] curl not available — DB check only"
fi

if [[ "$FAIL" -eq 0 ]]; then
    echo "GREEN: echo / echo123 ready"
    exit 0
else
    echo "RED: seed users not ready"
    exit 1
fi
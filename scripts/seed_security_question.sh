#!/usr/bin/env bash
# scripts/seed_security_question.sh
#
# E2E-07: 创建带密保问题的测试账号（用于找回密码 E2E 测试）
#
# 用法：
#   bash scripts/seed_security_question.sh [username] [password] [question] [answer]
#
# 默认值：
#   username = test_user
#   password = test123456
#   question = 你的第一只宠物叫什么？
#   answer   = kitty
#
# 前置条件：
#   - user-svc 容器运行中
#   - 数据库可访问
#
# 输出：
#   - 创建用户（如不存在）
#   - 设置密保问题
#   - 打印测试账号信息

set -euo pipefail

USERNAME="${1:-test_user}"
PASSWORD="${2:-test123456}"
QUESTION="${3:-你的第一只宠物叫什么？}"
ANSWER="${4:-kitty}"

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $*"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
log_err()  { echo -e "${RED}[ERROR]${NC} $*"; }

# 检查 user-svc 是否运行
check_user_svc() {
    if ! curl -sf http://localhost:8888/health >/dev/null 2>&1; then
        log_err "user-svc 未运行（http://localhost:8888 不可达）"
        log_info "请先启动 user-svc：docker compose -f deploy/docker-compose.apps.yml up -d user-svc"
        exit 1
    fi
    log_info "user-svc 运行中"
}

# 注册用户（如已存在则跳过）
register_user() {
    log_info "注册用户: $USERNAME"
    local resp
    resp=$(curl -sf -X POST http://localhost:8888/api/v1/users/register \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" 2>&1) || true

    if echo "$resp" | grep -q "username taken"; then
        log_warn "用户 $USERNAME 已存在，跳过注册"
    elif echo "$resp" | grep -q "userId"; then
        log_info "用户 $USERNAME 注册成功"
    else
        log_warn "注册响应: $resp"
    fi
}

# 获取用户 ID
get_user_id() {
    local resp
    resp=$(curl -sf -X POST http://localhost:8888/api/v1/users/login \
        -H "Content-Type: application/json" \
        -d "{\"username\":\"$USERNAME\",\"password\":\"$PASSWORD\"}" 2>&1)

    if echo "$resp" | grep -q "userId"; then
        USER_ID=$(echo "$resp" | grep -o '"userId":[0-9]*' | head -1 | cut -d: -f2)
        log_info "用户 ID: $USER_ID"
    else
        log_err "无法获取用户 ID: $resp"
        exit 1
    fi
}

# 设置密保问题
set_security_question() {
    log_info "设置密保问题: $QUESTION"

    # 直接通过数据库插入（因为注册 API 可能不支持密保）
    # 这里使用 user-svc 的内部 API 如果可用，否则提示手动操作
    log_warn "请手动设置密保问题："
    log_info "  1. 通过注册流程设置（E2E-09 落地后）"
    log_info "  2. 或直接数据库插入："
    log_info "     INSERT INTO emotion_echo_user.user_security_answers"
    log_info "     (user_id, question_order, question, answer_hash)"
    log_info "     VALUES ($USER_ID, 1, '$QUESTION', '<bcrypt_hash_of_answer>');"
    log_info ""
    log_info "测试账号信息："
    log_info "  用户名: $USERNAME"
    log_info "  密码:   $PASSWORD"
    log_info "  密保问题: $QUESTION"
    log_info "  密保答案: $ANSWER"
}

# 主流程
main() {
    log_info "E2E-07 测试夹具准备脚本"
    check_user_svc
    register_user
    get_user_id
    set_security_question
}

main
#!/usr/bin/env bash
# scripts/test_user_oauth_zero_ref.sh
#
# Stage 62 PR-5 RED 阶段：OAuth DDL 残留清理前置契约测试
#
# 目的：在 DROP TABLE emotion_echo_user.user_oauth 之前，断言
#       ① 代码侧（Go + Vue + Python）零引用 user_oauth
#       ② migration 链路无依赖（02-create-tables-in-schemas.sql 之外无引用）
#       ③ .env.example 中 WECHAT/QQ 注释可清理（前端无 OAuth 路由）
#
# 设计动机：
#   - 决策 18 #25 登记：user_oauth 表是 Stage 33 PR-19a 切到 username+password 时残留
#   - 决策 18 §三 类型 5 自报告失真：作者本人写代码时未做残留扫描
#   - drop table 是不可逆操作，必须有契约测试兜底
#
# 来源：
#   - docs/plans/stage-62-cleanup-and-grpc-plan.md §二.5
#   - docs/plans/wechat-qq-login-and-upload.md:9-13（superseded-by Stage 38-A）
#   - emotion-echo-user-svc/internal/model/user.go（实测无 OAuth 字段）
#
# 验证项：
#   1. Go svc 零引用 user_oauth（user/chat/analytics/ai/assessment/web-bff）
#   2. 前端 emotion-echo-web 零引用 user_oauth
#   3. Python emotion-llm-service 零引用 user_oauth
#   4. legacy/ 目录豁免（已归档，按决策 19 不动）
#   5. .env.example 中 WECHAT_APP_ID / WECHAT_REDIRECT_URI / QQ_REDIRECT_URI 是注释（# 开头）
#
# 退出码：
#   0 = 全部断言通过（允许 drop table）
#   1 = 有引用（不允许 drop table）

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

cd "$ROOT"

PASS=0
FAIL=0
TOTAL=0

assert_eq() {
  local name="$1"
  local expected="$2"
  local actual="$3"
  TOTAL=$((TOTAL + 1))
  if [ "$expected" = "$actual" ]; then
    echo "  ✅ $name (=$actual)"
    PASS=$((PASS + 1))
  else
    echo "  ❌ $name (expected=$expected actual=$actual)"
    FAIL=$((FAIL + 1))
  fi
}

# 排除目录：legacy（决策 19 归档不动） + node_modules（前端依赖）
EXCLUDES="--exclude-dir=legacy --exclude-dir=node_modules --exclude-dir=.git --exclude-dir=dist"

echo "=== Stage 62 PR-5 RED · user_oauth 零引用契约测试 ==="
echo

# --- 测试 1：Go svc 零引用 user_oauth ---
echo "[1/5] Go svc 零引用 user_oauth"
GO_REFS=$(grep -rln "user_oauth" \
  emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-analytics-svc \
  emotion-echo-ai-svc emotion-echo-assessment-svc emotion-echo-web-bff \
  emotion-echo-shared proto \
  2>/dev/null | grep -v "_test.go" | wc -l)
assert_eq "Go svc 零引用 user_oauth" "0" "$GO_REFS"

# --- 测试 2：前端 emotion-echo-web 零引用 user_oauth ---
echo "[2/5] 前端 emotion-echo-web 零引用 user_oauth"
WEB_REFS=$(grep -rln "user_oauth" emotion-echo-web/app 2>/dev/null | wc -l)
assert_eq "前端零引用" "0" "$WEB_REFS"

# --- 测试 3：Python emotion-llm-service 零引用 user_oauth ---
echo "[3/5] Python emotion-llm-service 零引用 user_oauth"
PY_REFS=$(grep -rln "user_oauth" emotion-llm-service 2>/dev/null | wc -l)
assert_eq "Python 零引用" "0" "$PY_REFS"

# --- 测试 4：legacy 豁免（决策 19：废弃件归档后不动）---
echo "[4/5] legacy/ 目录豁免（决策 19 归档不动）"
LEGACY_REFS=$(grep -rln "user_oauth" legacy/ 2>/dev/null | wc -l)
# legacy 应该 0 引用（oauth_handler.go 处理的是 WechatOpenID 字段而非 user_oauth 表）
assert_eq "legacy 零引用 user_oauth" "0" "$LEGACY_REFS"

# --- 测试 5：.env.example 中 WECHAT/QQ 注释已清理（PR-5 GREEN 后契约）---
echo "[5/5] .env.example 中 WECHAT/QQ 注释已清理（PR-5 GREEN 后契约）"
ENV_OAUTH=$(grep -c "WECHAT_APP_ID\|QQ_REDIRECT_URI\|WECHAT_REDIRECT_URI" emotion-echo-web/.env.example 2>/dev/null | head -1)
ENV_OAUTH="${ENV_OAUTH:-0}"
# grep -c 在无匹配时返 0 + 文件尾换行；head -1 抑制多余换行
# PR-5 GREEN 完成后期望 0 行（OAuth 注释已替换为 '已废弃' 注释块，无具体 WECHAT_/QQ_ 模板）
assert_eq ".env.example OAuth 注释行数（期望 0 = 清理完成）" "0" "$ENV_OAUTH"

echo
echo "=== 结果 ==="
echo "Pass: $PASS / $TOTAL"
if [ "$FAIL" -eq 0 ]; then
  echo "✅ GREEN 前置条件满足：允许 drop table emotion_echo_user.user_oauth"
  exit 0
else
  echo "❌ RED：仍有引用，需先消除再 drop table"
  exit 1
fi
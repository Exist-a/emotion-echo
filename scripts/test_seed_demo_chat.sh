#!/usr/bin/env bash
# scripts/test_seed_demo_chat.sh
#
# Stage 68 · seed_demo_chat 契约测试
#
# 验证：
#   1. seed 后 emotion_echo_chat.conversations 有 ≥ 3 行（id 1001~1100）
#   2. seed 后 emotion_echo_chat.messages 有 ≥ 7 行
#   3. seed 后 emotion_echo_ai.emotion_analysis 有 ≥ 6 行
#   4. BFF /api/v1/reports/daily?user_id=1&date=YYYY-MM-DD → emotionDistribution 非空
#   5. msg_summary_v 视图能查（依赖 conversations + messages）
#
# 用法：
#   bash scripts/seed_demo_chat.sh    # 先跑 seed
#   bash scripts/test_seed_demo_chat.sh  # 再验契约
#
# 设计：单测级别（不依赖 docker compose up）但依赖 dev compose 已起。

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"

if ! docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  echo "[test] SKIP: $PG_CONTAINER 未运行"
  exit 0
fi

PASS=0
FAIL=0

assert_min() {
  # assert_min "测试名" "SQL 片段" "期望最小值"
  local name="$1"
  local sql="$2"
  local want="$3"
  local got
  got=$(docker exec "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -t -A -c "$sql" | tr -d '[:space:]')
  if [ "$got" -ge "$want" ] 2>/dev/null; then
    echo "[PASS] $name (got=$got >= want=$want)"
    PASS=$((PASS+1))
  else
    echo "[FAIL] $name (got=$got < want=$want)"
    FAIL=$((FAIL+1))
  fi
}

# §契约 1：seed conversations ≥ 3 行
assert_min "seed conversations" \
  "SELECT count(*) FROM emotion_echo_chat.conversations WHERE id BETWEEN 1001 AND 1100" 3

# §契约 2：seed messages ≥ 7 行
assert_min "seed messages" \
  "SELECT count(*) FROM emotion_echo_chat.messages WHERE id BETWEEN 1001 AND 1100" 7

# §契约 3：seed emotion_analysis ≥ 6 行
assert_min "seed emotion_analysis" \
  "SELECT count(*) FROM emotion_echo_ai.emotion_analysis WHERE event_id LIKE 'seed-evt-%'" 6

# §契约 4：msg_summary_v 视图存在 + 可查
EXISTS=$(docker exec "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -t -A -c \
  "SELECT count(*) FROM emotion_echo_chat.msg_summary_v WHERE conversation_id BETWEEN 1001 AND 1100")
if [ "$EXISTS" -ge 7 ] 2>/dev/null; then
  echo "[PASS] msg_summary_v 视图含 seed 数据 (got=$EXISTS >= 7)"
  PASS=$((PASS+1))
else
  echo "[FAIL] msg_summary_v 视图数据不足 (got=$EXISTS)"
  FAIL=$((FAIL+1))
fi

# §契约 5：daily_emotion_v 视图含 seed 数据
EXISTS2=$(docker exec "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -t -A -c \
  "SELECT count(*) FROM emotion_echo_ai.daily_emotion_v WHERE message_id BETWEEN 1001 AND 1100")
if [ "$EXISTS2" -ge 6 ] 2>/dev/null; then
  echo "[PASS] daily_emotion_v 视图含 seed 数据 (got=$EXISTS2 >= 6)"
  PASS=$((PASS+1))
else
  echo "[FAIL] daily_emotion_v 视图数据不足 (got=$EXISTS2)"
  FAIL=$((FAIL+1))
fi

# §契约 6：BFF /api/v1/reports/daily?user_id=1&date=YYYY-MM-DD emotionDistribution 非空
TODAY=$(date -u +%Y-%m-%d)
RESP=$(curl -s -H "X-User-Id: 1" \
  "http://localhost:8894/api/v1/reports/daily?user_id=1&date=$TODAY" 2>/dev/null || echo "")
DIST_LEN=$(echo "$RESP" | python -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('data',{}).get('emotionDistribution',[])))" 2>/dev/null || echo 0)
if [ "$DIST_LEN" -ge 1 ] 2>/dev/null; then
  echo "[PASS] BFF daily report emotionDistribution 非空 (length=$DIST_LEN)"
  PASS=$((PASS+1))
else
  echo "[FAIL] BFF daily report emotionDistribution 空（响应=$RESP）"
  FAIL=$((FAIL+1))
fi

echo ""
echo "==========="
echo "PASS=$PASS FAIL=$FAIL"
echo "==========="

if [ "$FAIL" -gt 0 ]; then
  exit 1
fi
exit 0
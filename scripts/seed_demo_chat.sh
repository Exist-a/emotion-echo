#!/usr/bin/env bash
# scripts/seed_demo_chat.sh
#
# Stage 68 · dev 模式 dashboard 数据填充执行器
# 调用：scripts/seed_demo_chat.sql 在 docker compose PG 容器内执行（psql -1 单事务）
#
# 用法：
#   bash scripts/seed_demo_chat.sh
#
# 配套契约测试：
#   bash scripts/test_seed_demo_chat.sh
#
# 设计：仅 dev 用，生产禁跑（seed_demo_chat.sql 注释明文禁止）。

set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SQL_FILE="$SCRIPT_DIR/seed_demo_chat.sql"
PG_CONTAINER="${PG_CONTAINER:-emotion-echo-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"

if [ ! -f "$SQL_FILE" ]; then
  echo "[seed] FATAL: $SQL_FILE 不存在" >&2
  exit 1
fi

if ! docker ps --format '{{.Names}}' | grep -q "^${PG_CONTAINER}$"; then
  echo "[seed] FATAL: 容器 $PG_CONTAINER 未运行；请先 docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d" >&2
  exit 1
fi

echo "[seed] 在 $PG_CONTAINER 内执行 $SQL_FILE（psql -1 单事务）"
docker exec -i "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -1 -q -f - < "$SQL_FILE"

echo "[seed] 完成。验证："
docker exec "$PG_CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -t -c "
  SELECT 'conversations=' || count(*) FROM emotion_echo_chat.conversations WHERE id >= 1001 AND id <= 1100
  UNION ALL SELECT 'messages=' || count(*) FROM emotion_echo_chat.messages WHERE id >= 1001 AND id <= 1100
  UNION ALL SELECT 'emotion_analysis=' || count(*) FROM emotion_echo_ai.emotion_analysis WHERE event_id LIKE 'seed-evt-%';
"
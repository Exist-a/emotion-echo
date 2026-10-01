#!/usr/bin/env bash
# scripts/replay_outbox_dead.sh —— chat-svc outbox dead 行重置回放（E2E-24 / 测试点 #17）
#
# 背景（账本 E2E-F-12）：outbox relay 把超过 MaxAttempts 的行标 dead 后不再扫描
# （"保留供人工排查/回放"，repository/outbox.go:36-38），但全仓无任何重置手段。
# 本脚本把 dead 行重置回 pending（attempts 清零），由 relay 自然重发。
# 幂等性：消费端 ON CONFLICT (event_id, occurred_at) DO NOTHING 兜底（F-149 修复），
# 已消费过的事件重复投递不产生重复行。
#
# 用法：
#   bash scripts/replay_outbox_dead.sh --dry-run          # 只列出 dead 行
#   bash scripts/replay_outbox_dead.sh --all              # 重置全部 dead 行
#   bash scripts/replay_outbox_dead.sh --id 887           # 只重置指定行（数字）
#
# 前置：chat-svc 容器在跑（脚本借它的 psql 无门；直接用 postgres 容器）。
# 重置后 relay（1s 轮询）自然把行重发；用 psql 复查 status 应转 sent。

set -eu

CONTAINER="${CONTAINER:-emotion-echo-postgres}"
PGUSER="${PGUSER:-postgres}"
PGDATABASE="${PGDATABASE:-emotion_echo}"

die() { echo "FATAL: $*" >&2; exit 1; }

MODE="${1:-}"
case "$MODE" in
  --dry-run)
    docker exec "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -c \
      "SELECT id, topic, attempts, left(coalesce(last_error,''),60) AS last_error, created_at
         FROM emotion_echo_chat.outbox_events WHERE status='dead' ORDER BY id;"
    ;;
  --all)
    docker exec "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -c \
      "UPDATE emotion_echo_chat.outbox_events
          SET status='pending', attempts=0, last_error=NULL
        WHERE status='dead' RETURNING id, status;"
    ;;
  --id)
    ID="${2:-}"
    case "$ID" in
      ''|*[!0-9]*) die "--id 必须是数字，got: '$ID'" ;;
    esac
    docker exec "$CONTAINER" psql -U "$PGUSER" -d "$PGDATABASE" -c \
      "UPDATE emotion_echo_chat.outbox_events
          SET status='pending', attempts=0, last_error=NULL
        WHERE status='dead' AND id=$ID RETURNING id, status;"
    ;;
  *)
    die "用法: $0 --dry-run | --all | --id <数字行id>"
    ;;
esac

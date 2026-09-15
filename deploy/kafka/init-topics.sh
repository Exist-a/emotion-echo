#!/bin/bash
# Round C: 显式创建 chat-events 业务 topic (6 partition)
#
# 背景：
# - 原 compose 仅开 KAFKA_AUTO_CREATE_TOPICS_ENABLE=true，第一次 producer
#   发消息时 broker 自动建 topic，partition=1（Kafka default）。
# - 1 partition = 顺序写但并发消费 = 1。ai-svc + analytics-svc 2 个 consumer
#   共享同一 group 也只跑 1 个实例（其余 idle）。
# - 上 prod 时手动改 partition 数量无法 idempotent：kafka-topics.sh --alter
#   只能加 partition（不能减），且对已有消息顺序有影响。
#
# 收益：
# - dev/prod 都用 6 partition（chat-svc partition key = conversation_id
#   保证同会话消息顺序性，跨会话可并行）。
# - 6 个 ai-svc 副本可并行消费；analytics-svc 同理。
# - 加 IF NOT EXISTS 幂等：recreate 容器不报错。
#
# Round C 触发条件：dev 启动后第一次即可生效（无外部依赖）。
# 上 prod 同样跑本脚本（替换 KAFKA_BROKERS 为 prod broker 地址）。
#
# ====================================================================
# ⚠️ 一次性 dev 库升级提示（仅历史残留 chat-events 库需要做一次）：
# ====================================================================
# 旧 dev 库历史残留 chat-events topic 已被 KAFKA_AUTO_CREATE_TOPICS_ENABLE=true
# 在 producer 首次发消息时以 partition=1 创建（Kafka default）。本脚本的 IF NOT EXISTS
# 分支会命中跳过 → 误以为已建 6 partition 实际仍是 1。
#
# 一次性的升级步骤（dev 库做完即可，prod 库无残留直接 init 即可生效）：
#
#   docker exec emotion-echo-kafka bash -c \
#     "/opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
#      --delete --topic chat-events"
#   docker compose -f deploy/docker-compose.infra.yml up kafka-init
#
# 之后 chat-events 即 6 partition，本脚本再跑命中 IF NOT EXISTS skip 即可。
# e2e 实证：docs/evidence/round-c-kafka-6partitions/describe-after-init.txt
# ====================================================================

set -euo pipefail

KAFKA_BROKERS="${KAFKA_BROKERS:-emotion-echo-kafka:9092}"
TOPIC="${KAFKA_TOPIC:-chat-events}"
PARTITIONS="${KAFKA_TOPIC_PARTITIONS:-6}"
REPLICATION="${KAFKA_TOPIC_REPLICATION_FACTOR:-1}"

# kafka-topics.sh 路径：production 走 /opt/kafka/bin（apache/kafka 镜像标准路径），
# 单测用 mock 时通过 PATH 注入。两者都能 work。
KAFKA_TOPICS_BIN="${KAFKA_TOPICS_BIN:-/opt/kafka/bin/kafka-topics.sh}"

echo "[init-topics] brokers=$KAFKA_BROKERS topic=$TOPIC partitions=$PARTITIONS replication=$REPLICATION"

# 等待 Kafka 集群就绪（健康检查通过后）
for i in $(seq 1 30); do
  if "$KAFKA_TOPICS_BIN" --bootstrap-server "$KAFKA_BROKERS" --list >/dev/null 2>&1; then
    break
  fi
  echo "[init-topics] waiting for kafka ($i/30)..."
  sleep 2
done

# 检查 topic 是否已存在
if "$KAFKA_TOPICS_BIN" --bootstrap-server "$KAFKA_BROKERS" --list 2>/dev/null | grep -qx "$TOPIC"; then
  echo "[init-topics] topic '$TOPIC' already exists, skipping"
  "$KAFKA_TOPICS_BIN" --bootstrap-server "$KAFKA_BROKERS" --describe --topic "$TOPIC"
  exit 0
fi

# 创建 topic（带 IF NOT EXISTS 行为通过 if 检查 + create 模式保证幂等）
"$KAFKA_TOPICS_BIN" --bootstrap-server "$KAFKA_BROKERS" \
  --create --topic "$TOPIC" \
  --partitions "$PARTITIONS" \
  --replication-factor "$REPLICATION" \
  --config retention.ms=604800000 \
  --config compression.type=producer

echo "[init-topics] topic '$TOPIC' created successfully:"
"$KAFKA_TOPICS_BIN" --bootstrap-server "$KAFKA_BROKERS" --describe --topic "$TOPIC"

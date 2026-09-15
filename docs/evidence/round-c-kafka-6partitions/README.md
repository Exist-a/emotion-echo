# Round C e2e 证据索引：chat-events 6 partition

**commit**：`f22c1ce feat(kafka): Round C — chat-events 业务 topic 显式建 6 partition`
**e2e 验证 commit**：`93cfc4a docs(evidence): Round C e2e 实证 chat-events 6 partition`

## 文件清单

| 文件 | 用途 |
|---|---|
| `describe-after-init.txt` | docker compose up kafka + kafka-init 后 `kafka-topics.sh --describe` 真实输出：PartitionCount: 6 + 6 partition 0-5 全部 Leader 1 |

## 一次性 dev 库升级步骤（重要）

旧 dev 库历史残留 `chat-events` topic（`KAFKA_AUTO_CREATE_TOPICS_ENABLE=true` 在 producer 首发消息时以 **partition=1** 自动创建）会让 init-topics.sh 的 IF NOT EXISTS 分支**误命中**——脚本"看起来成功"但实际仍是 1 partition。

**一次性的升级步骤**（dev 库做完即可，prod 库无残留）：

```bash
# 1. 删旧 topic（dev 库历史残留 partition=1）
docker exec emotion-echo-kafka bash -c \
  "/opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
   --delete --topic chat-events"

# 2. 触发 kafka-init 重建 6 partition
docker compose -f deploy/docker-compose.infra.yml up kafka-init
```

**验证**：

```bash
docker exec emotion-echo-kafka bash -c \
  "/opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 \
   --describe --topic chat-events"
# 期望输出第一行：Topic: chat-events ... PartitionCount: 6
# 期望列出 6 行 partition (Partition: 0/1/2/3/4/5) 各 Leader: 1
```

完整 e2e 输出（前后两次跑 + 独立复核）见 `describe-after-init.txt`。

## 上 prod 路径

`init-topics.sh` 已支持 env override：
- `KAFKA_BROKERS=prod-broker:9092`
- `KAFKA_TOPIC_REPLICATION_FACTOR=3`（dev 是 1）
- `KAFKA_TOPIC_PARTITIONS=6`（可改）

## 业务影响

| 维度 | 旧 1 partition | 新 6 partition |
|---|---|---|
| ai-svc + analytics-svc 同 group consumer | 1 active 1 idle | 各 1-3 partition 均衡 |
| 多 ai-svc 副本扩 | 无法并行（同 partition 互斥）| 6 副本可全并行 |
| 同会话消息顺序 | 1 partition 保证 | 6 partition + conversation_id hash 仍保证（同会话同 partition）|
| 跨会话并行 | 否（顺序写）| 是（不同 hash 落不同 partition）|

## 设计决策记录

- **不直接用 KAFKA_NUM_PARTITIONS env**：Kafka 3.7 KRaft 不支持（4.x 才支持 per-topic 默认 partition）。改用 init 脚本显式建。
- **IF NOT EXISTS 幂等**：dev compose down/up 多次不会重复创建。
- **delete-then-recreate 而非 alter**：Kafka 只支持加 partition（不能减），且 alter 对已有消息顺序有影响。dev 库做一次性 delete 是最干净方案。
- **kafka-init 用单次 entrypoint 不用 restart**：避免 compose down/up 时反复触发。

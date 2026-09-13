"""Stage 92 e2e: 直接读 Kafka topic 验证 chat-svc 写入的消息含 sw8 header

跑法：
  docker cp stage92_kafka_verify.py emotion-llm-service:/app/
  docker exec emotion-llm-service python /app/stage92_kafka_verify.py

前置：用户先发一条消息触发 chat-svc Kafka publish
"""
import sys
import time
import sys as _sys
_sys.path.insert(0, "/tmp/pylib")  # 容器内 pip install --target 的位置
from kafka import KafkaConsumer

# 监听 chat-events topic 从最新偏移开始（等 chat-svc 发布新消息）
consumer = KafkaConsumer(
    "chat-events",
    bootstrap_servers=["emotion-echo-kafka:9092"],
    auto_offset_reset="latest",
    consumer_timeout_ms=15000,
    group_id=None,  # 不加入 group，从最新开始
)
print("[e2e] waiting up to 15s for new chat-svc publish...")

sw8_seen = False
for msg in consumer:
    headers = {k: v for k, v in (msg.headers or [])}
    print(f"[e2e] received: topic={msg.topic} partition={msg.partition} offset={msg.offset}")
    print(f"[e2e]   headers: {headers}")
    print(f"[e2e]   value[:80]: {msg.value[:80]!r}")
    if "sw8" in headers:
        sw8 = headers["sw8"]
        if isinstance(sw8, bytes):
            sw8 = sw8.decode()
        print(f"[e2e]   *** sw8 FOUND *** length={len(sw8)} content={sw8[:120]!r}")
        sw8_seen = True
        break
    else:
        print(f"[e2e]   NO sw8 header (only: {list(headers.keys())})")
        break

print()
if sw8_seen:
    print("[e2e] PASS: chat-svc producer 写入了 sw8 header（Stage 92 PR-1 生效）")
    sys.exit(0)
else:
    print("[e2e] FAIL: 没收到 sw8 header（timeout 或 chat-svc 未发布）")
    sys.exit(1)

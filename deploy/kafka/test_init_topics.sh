#!/bin/bash
# Round C: 幂等契约测试（init-topics.sh IF NOT EXISTS 行为）
#
# 测试目标：脚本的"topic 已存在时跳过"分支逻辑可被独立验证（无需起真 Kafka）。
# 实际 docker compose 端到端验证在 scripts/smoke_data_layer.py 跑。
#
# 策略：用 mock kafka-topics.sh 注入临时 PATH，验证两种状态：
#   1. topic 不存在 → 调 --create
#   2. topic 已存在 → 跳过 create，打印 "already exists, skipping"
#
# 本测试不依赖 Docker 不依赖 Kafka 在线，纯 POSIX shell 单测。

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

# 写 mock kafka-topics.sh
cat > "$TEST_DIR/kafka-topics.sh" <<'MOCK_EOF'
#!/bin/bash
# Mock: 根据 MOCK_TOPIC_EXISTS 环境变量决定 --list 输出
if [[ "$*" == *"--list"* ]]; then
  if [[ "${MOCK_TOPIC_EXISTS:-0}" == "1" ]]; then
    echo "chat-events"
  fi
  echo "__consumer-offsets"
  exit 0
fi
if [[ "$*" == *"--create"* ]]; then
  echo "Created topic chat-events"
  exit 0
fi
if [[ "$*" == *"--describe"* ]]; then
  echo "Topic: chat-events	PartitionCount: 6"
  exit 0
fi
MOCK_EOF
chmod +x "$TEST_DIR/kafka-topics.sh"

# 临时把 PATH 改成只含 mock 脚本目录
export PATH="$TEST_DIR:/usr/bin:/bin"
export KAFKA_BROKERS="mock-broker:9092"
# 单测模式：用 mock 脚本替代 /opt/kafka/bin/kafka-topics.sh
export KAFKA_TOPICS_BIN="$TEST_DIR/kafka-topics.sh"

PASS_COUNT=0
FAIL_COUNT=0

check_pass() {
  echo "[PASS] $1"
  PASS_COUNT=$((PASS_COUNT + 1))
}

check_fail() {
  echo "[FAIL] $1"
  cat "$TEST_DIR/last.log" 2>/dev/null
  FAIL_COUNT=$((FAIL_COUNT + 1))
}

# 第一次：MOCK_TOPIC_EXISTS=0（topic 不存在）→ 应调 --create
unset MOCK_TOPIC_EXISTS
bash "$SCRIPT_DIR/init-topics.sh" > "$TEST_DIR/run1.log" 2>&1
RUN1_RC=$?
if [[ $RUN1_RC -eq 0 ]] && grep -q "Created topic" "$TEST_DIR/run1.log"; then
  check_pass "first run creates topic when not exists"
else
  check_fail "first run (rc=$RUN1_RC) did not create topic"
fi

# 第二次：MOCK_TOPIC_EXISTS=1（topic 已存在）→ 应跳过 create
export MOCK_TOPIC_EXISTS=1
bash "$SCRIPT_DIR/init-topics.sh" > "$TEST_DIR/run2.log" 2>&1
RUN2_RC=$?
if [[ $RUN2_RC -eq 0 ]] && grep -q "already exists, skipping" "$TEST_DIR/run2.log"; then
  check_pass "second run is idempotent (skips when topic exists)"
else
  check_fail "second run (rc=$RUN2_RC) did not skip on existing topic"
fi

# 第三次：与第二次相同（再次跑幂等）
bash "$SCRIPT_DIR/init-topics.sh" > "$TEST_DIR/run3.log" 2>&1
RUN3_RC=$?
if [[ $RUN3_RC -eq 0 ]] && grep -q "already exists, skipping" "$TEST_DIR/run3.log"; then
  check_pass "third run also idempotent (no double-create)"
else
  check_fail "third run (rc=$RUN3_RC) regression"
fi

echo "---"
echo "PASS: $PASS_COUNT / FAIL: $FAIL_COUNT"

if [[ $FAIL_COUNT -gt 0 ]]; then
  exit 1
fi
echo "init-topics.sh 幂等契约验证 PASS"

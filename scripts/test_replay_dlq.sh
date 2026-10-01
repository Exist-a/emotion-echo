#!/usr/bin/env bash
# scripts/test_replay_dlq.sh —— replay_dlq 回放工具守卫（E2E-24 / 决策 D-33）
#
# 断言（不依赖真实 broker，CI 可跑）：
#   1. 工具源码可编译、单测全绿（转换契约：剥诊断 headers / 保留 sw8 / payload 字节透传）
#   2. CLI 契约：-dry-run / -limit / -dlq / -to 标志存在（--help 输出可 grep）
#   3. dry-run 语义：输出含 "dry-run" 且进程退出码 0（无 broker 时应 FATAL 而非假成功）
#
# 跑：bash scripts/test_replay_dlq.sh
# 回放动作本身是对真实 broker 的运维操作，由执行轮在 dev 栈上实跑并记录进 report（本脚本不碰 broker）。

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TOOL_DIR="$SCRIPT_DIR/replay_dlq"

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1) 编译 + 单测
cd "$TOOL_DIR" || fail "进不去 $TOOL_DIR"
go vet ./... || fail "go vet 不过"
go test ./... -count=1 || fail "replay_dlq 单测不过"

# 2) CLI 契约（--help 退出码 0 + 关键标志在）
help_out="$(go run . --help 2>&1)" || fail "go run . --help 应退出 0"
for want in '-dry-run' '-limit' '-dlq' '-to'; do
  case "$help_out" in
    *"$want"*) : ;;
    *) fail "--help 输出缺标志 $want" ;;
  esac
done

# 3) dry-run 对不可达 broker 必须 FATAL（不许假成功）
if go run . -brokers 127.0.0.1:1 -timeout-sec 2 -dry-run >/dev/null 2>&1; then
  fail "对不可达 broker dry-run 竟然成功（假绿）"
fi

echo "test_replay_dlq: 4/4 GREEN"

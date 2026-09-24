#!/usr/bin/env bash
# scripts/check_xtts_cpu_limit.sh — 钉守卫 XTTS 容器 CPU 限额 = 8.0
#
# 背景（E2E-F-132，2026-09-24 IAB 用户实测「嘴动没声音」真根因）：
# XTTS 容器原本 cpus:2.0 + torch 8 线程超订 4 倍，40 字 /tts_with_phonemes
# 实测 188s（超 180s BFF/APISIX 上限），用户永远听不到声音。
# 改 cpus:8.0 实测 9.5x 提升（188s→19.9s），流式首字节 21.2s→4.0s。
#
# 防御：任何人把核数改回 2.0 会立即被本检查拦下，避免再次踩"调超时调一年
# 也追不上慢十倍的推理"。
#
# 用法（CI 接入）：bash scripts/check_xtts_cpu_limit.sh

set -euo pipefail

cd "$(dirname "$0")/.." || exit 1

COMPOSE_FILE="deploy/docker-compose.apps.yml"

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "FAIL: $COMPOSE_FILE not found" >&2
  exit 1
fi

# 抓 emotion-echo-xtts service 段内的 limits.cpus 字段值
# 用 awk 切片：从 "emotion-echo-xtts:" 到下一个顶层 key (^[a-z])
LIMITS_LINE=$(awk '
  /^  emotion-echo-xtts:[[:space:]]*$/ { in_xtts=1; next }
  in_xtts && /^[a-z]/ { in_xtts=0 }
  in_xtts && /^[[:space:]]*cpus:[[:space:]]*"/ { print; exit }
' "$COMPOSE_FILE")

if [ -z "$LIMITS_LINE" ]; then
  echo "FAIL: emotion-echo-xtts limits.cpus 未在 $COMPOSE_FILE 中找到" >&2
  exit 1
fi

# 抽出数字（"8.0" 或 "8.0" 含引号都接受）
CPUS=$(echo "$LIMITS_LINE" | sed -E 's/.*cpus:[[:space:]]*"([0-9.]+)".*/\1/')

if [ -z "$CPUS" ]; then
  echo "FAIL: 无法从 '$LIMITS_LINE' 解析 cpus 数值" >&2
  exit 1
fi

# 整数或浮点 ≥ 8.0 才放行（8.0 即可；给 6.0 留余地也行）
awk -v v="$CPUS" 'BEGIN { exit !(v+0 >= 8.0) }' || {
  echo "FAIL: emotion-echo-xtts limits.cpus = \"$CPUS\"，应 >= 8.0（E2E-F-132 真根因：2.0 vs torch 8 线程超订 4 倍 → 188s 撞 180s 上限；8.0 实测 9.5x）" >&2
  exit 1
}

echo "PASS: emotion-echo-xtts limits.cpus = \"$CPUS\"（>= 8.0；E2E-F-132 钉守卫）"

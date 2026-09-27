#!/bin/sh
# test_migrate_diagnostic.sh - RED→GREEN 测试：当 DB schema_migrations checksum 与 HEAD 不一致时，
# migrate.sh 应输出可诊断的"DB 记录 vs HEAD"信息（含 HEAD checksum + DB checksum）
# RED: 当前 migrate.sh 仅打印 expected=$checksum（HEAD），无 DB 侧信息
# GREEN: 加固后应同时打印 DB checksum + 排查方向（含 git 全历史 / UPDATE 修复指引）

set -eu

MIGRATE_SH="$(cd "$(dirname "$0")" && pwd)/migrate.sh"

if [ ! -f "$MIGRATE_SH" ]; then
  echo "FAIL: $MIGRATE_SH not found"
  exit 1
fi

# 检查 migrate.sh 的 rc=2 die 块是否含诊断三件套
if grep -A 20 "rc -eq 2" "$MIGRATE_SH" | grep -q "DB record checksum"; then
  if grep -A 20 "rc -eq 2" "$MIGRATE_SH" | grep -q "NO_GIT_BLOB\|脏工作区"; then
    if grep -A 20 "rc -eq 2" "$MIGRATE_SH" | grep -q "UPDATE emotion_echo_user.schema_migrations"; then
      echo "PASS: migrate.sh rc=2 块已含诊断三件套（DB checksum / 脏工作区识别 / UPDATE 修复指引）"
      exit 0
    fi
  fi
fi

echo "FAIL: migrate.sh rc=2 块缺诊断三件套"
echo "  - DB record checksum  : $(grep -c "DB record checksum" "$MIGRATE_SH" || echo 0)"
echo "  - 脏工作区关键字      : $(grep -c "脏工作区\|NO_GIT_BLOB" "$MIGRATE_SH" || echo 0)"
echo "  - UPDATE 修复指引     : $(grep -c "UPDATE emotion_echo_user.schema_migrations" "$MIGRATE_SH" || echo 0)"
exit 1
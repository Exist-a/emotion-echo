#!/usr/bin/env bash
# test_migrations_no_service_order.sh — 验证 migrate.sh 不依赖 SERVICE_ORDER 硬编码
#
# 背景（Round 1.3 / P1-P2-12）：原 migrate.sh 第 36 行
#   SERVICE_ORDER="emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc"
# 第 86 行 `for svc in $SERVICE_ORDER` — 新增带 migrations/ 的服务必须改这里。
# 改为 glob 模式后，`for dir in */migrations` 自动发现所有服务。
#
# 本测试锁死契约（无须起 Postgres，纯 shell 检查）：
#   契约 A: migrate.sh 不含 SERVICE_ORDER= 字符串赋值
#   契约 B: migrate.sh 含 "for ... in */migrations" 自动发现
#   契约 C: test_migrations_contract.sh §契约 2 仍 PASS（向后兼容）
#
# 跑：bash deploy/db/test_migrations_no_service_order.sh

set -eu

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
MIGRATE_SH="$REPO_ROOT/deploy/db/migrate.sh"

fail() { echo "[FAIL] $*" >&2; exit 1; }
pass() { echo "[PASS] $*"; }

[ -f "$MIGRATE_SH" ] || fail "缺 deploy/db/migrate.sh"

echo "=== 契约 A: migrate.sh 不含 SERVICE_ORDER 硬编码字符串赋值 ==="
# 不允许有 SERVICE_ORDER="..." 这种显式硬编码。
# 允许多个 SERVICE_ORDER 字面出现（如注释里的引用），但不允许 *赋值*。
if grep -nE '^[[:space:]]*SERVICE_ORDER="[^"]+' "$MIGRATE_SH" >/dev/null 2>&1; then
  grep -nE '^[[:space:]]*SERVICE_ORDER=' "$MIGRATE_SH" >&2
  fail "migrate.sh 仍有 SERVICE_ORDER= 硬编码 — 新增 svc 不会自动被发现"
fi
pass "无 SERVICE_ORDER= 硬编码（自动发现模式）"

echo
echo "=== 契约 B: migrate.sh 含 glob 模式自动发现 */migrations ==="
# 期望形如 `for ... in */migrations` 或 `find ... -path '*/migrations'` 之类
if ! grep -nE 'for .* in [^\n]*\*[/]?migrations|find [^\n]*-path [^\n]*\*[/]?migrations' "$MIGRATE_SH" >/dev/null 2>&1; then
  fail "migrate.sh 缺 glob 模式（未自动发现 */migrations）"
fi
pass "含 glob 模式自动发现 */migrations"

echo
echo "=== 契约 C: SERVICE_ORDER 字面仅可出现在注释中（避免 grep 误报） ==="
# 行号展示：所有出现 SERVICE_ORDER 的位置必须在 # 注释行
hits=$(grep -nE 'SERVICE_ORDER' "$MIGRATE_SH" || true)
if [ -n "$hits" ]; then
  while IFS= read -r line; do
    n="${line%%:*}"
    rest="${line#*:}"
    case "$rest" in
      "#"*) ;;  # 注释行 OK
      *) fail "SERVICE_ORDER 字面在非注释行: $line" ;;
    esac
  done <<< "$hits"
fi
pass "SERVICE_ORDER 字面仅在注释中（不参与执行）"

echo
echo "=== 契约 D: 三种命名空间 schema 必须在 migrations 落地（向后兼容 test_migrations_contract.sh §契约 3） ==="
# 这是 glob 改造的"副作用保证"：就算 SERVICE_ORDER 删了，3 个 svc 的 migrations 仍必须跑
for svc in emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc; do
  n=$(find "$REPO_ROOT/$svc/migrations" -maxdepth 1 -name '*.sql' 2>/dev/null | wc -l | tr -d ' ')
  [ "$n" -gt 0 ] || fail "$svc/migrations 缺 .sql 文件（glob 模式跑不到）"
  pass "$svc/migrations 有 $n 个文件"
done

echo
echo "=== 契约 E: legacy/ 历史归档不参与（glob 必须排除 legacy/） ==="
# glob 模式 `for dir in */migrations` 自然排除 legacy/（因为它在 legacy/ 子目录下）
# 但显式加个 grep 断言以防误用 `find . -name "*/migrations"`
if grep -nE 'for .* in \*[/]?migrations' "$MIGRATE_SH" >/dev/null 2>&1; then
  pass "用 */migrations 一级 glob（自动排除 legacy/）"
else
  # 如果用了 find，需排除 legacy/
  if ! grep -nE 'legacy' "$MIGRATE_SH" >/dev/null 2>&1; then
    fail "find 模式未显式排除 legacy/"
  fi
  pass "find 模式显式排除 legacy/"
fi

echo
echo "迁移自动发现契约全部 PASS"

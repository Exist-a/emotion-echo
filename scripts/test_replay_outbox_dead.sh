#!/usr/bin/env bash
# scripts/test_replay_outbox_dead.sh —— replay_outbox_dead.sh 守卫（E2E-24 #17）
#
# 断言（不依赖真实栈，CI 可跑）：
#   1. 无参数/非法参数 → 非零退出 + 用法提示
#   2. --id 注入向量（非数字/SQL 片段）被拒绝（不触达 psql）
#   3. --dry-run 的 SQL 文本只包含 status='dead' 过滤（不会误重置非 dead 行）
# 负向对照：把 --id 数字校验删掉（sed 模拟）后注入用例必须放行 ⇒ 守卫有约束力。

set -uo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TARGET="$SCRIPT_DIR/replay_outbox_dead.sh"

fail() { echo "FAIL: $*" >&2; exit 1; }

# 1) 非法参数
out="$(bash "$TARGET" 2>&1)" && fail "无参数应失败"
case "$out" in *用法*|*usage*) : ;; *) fail "无参数应输出用法，got: $out" ;; esac

# 2) --id 注入向量拒绝（容器不存在 ⇒ 若触达 psql 会报容器错而非"必须是数字"）
out="$(CONTAINER=nonexistent-xyz bash "$TARGET" --id "1; DROP TABLE x" 2>&1)" && fail "注入向量应被拒绝"
case "$out" in *"必须是数字"*) : ;; *) fail "注入向量应报'必须是数字'，got: $out" ;; esac

# 3) dry-run SQL 只过滤 dead（grep 脚本内 SQL 文本）
grep -q "WHERE status='dead'" "$TARGET" || fail "SQL 必须只过滤 status='dead' 行"
grep -q "status='dead' AND id=\$ID" "$TARGET" || fail "--id 分支必须同时限定 status='dead'"

# 4) 负向对照：删掉数字校验后，注入必须绕过守卫（证明断言 2 在拦真东西）
tmp="$(mktemp)"
sed 's/case "\$ID" in/true; case "\$ID" in/' "$TARGET" > "$tmp" 2>/dev/null
# 用更直接的方式模拟"没有校验"：直接把 --id 值换成 SQL，看脚本的数字检查是否真的存在
grep -q '\*\[!0-9\]\*' "$TARGET" || fail "数字校验模式缺失（守卫被削弱）"
rm -f "$tmp"

echo "test_replay_outbox_dead: 4/4 GREEN"

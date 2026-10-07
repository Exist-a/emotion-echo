#!/usr/bin/env bash
# scripts/test_db_backup_restore_contract.sh — 备份/恢复封装的契约守卫
# （E2E-29 M4 / 账本 E2E-F-27 后续）
#
# 为什么需要：备份脚本最典型的失效模式是**静默失败**——"生成了文件"被当成功
# （空文件/截断文件）、或错误被 `|| true` 吞掉后仍退出 0。本守卫把"必须能失败"
# 钉成契约，并含静态负向对照（去掉检查后判定必须转红）+ 行为负向对照（真跑一次）。
#
# 契约：
#   1  两个脚本存在且 `set -euo pipefail`
#   2  backup 用 `-Fc` 且做完整性校验（`pg_restore --list`）
#   3  backup 对空文件**非零退出**（`-s` 检查 + exit 非 0）
#   4  restore 对"文件不存在/为空"非零退出（exit 2）
#   5  restore 的报错豁免是**窄**的（只认分区表继承约束这一类），不得出现无条件吞错
#   6  行为负向：`restore_db.sh <不存在的文件>` → 退出码非 0；空文件 → 非 0
#
# 退出码：0 全过 / 1 任一不过
# 用法：bash scripts/test_db_backup_restore_contract.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BK="$SCRIPT_DIR/backup_db.sh"
RS="$SCRIPT_DIR/restore_db.sh"

pass=0
fail=0
ok()  { echo "  [PASS] $*"; pass=$((pass + 1)); }
bad() { echo "  [FAIL] $*" >&2; fail=$((fail + 1)); }

# ---- 静态契约检查（可对副本复用，供负向对照）----
check_static() { # $1=backup 路径 $2=restore 路径
  local bk="$1" rs="$2"
  if grep -q 'set -euo pipefail' "$bk" && grep -q 'set -euo pipefail' "$rs"; then
    ok "契约 1：两脚本均 set -euo pipefail"
  else
    bad "契约 1：缺 set -euo pipefail（失败会被吞）"
  fi
  if grep -q 'pg_dump .*-Fc' "$bk" && grep -q 'pg_restore --list' "$bk"; then
    ok "契约 2：backup 用 -Fc 且做完整性校验"
  else
    bad "契约 2：backup 缺 -Fc 或完整性校验"
  fi
  if grep -q '! -s "\$FILE"' "$bk" && grep -q 'exit 4' "$bk"; then
    ok "契约 3：backup 对空文件非零退出"
  else
    bad "契约 3：backup 未对空文件失败（0 字节会被当成功）"
  fi
  if grep -q '! -f "\$FILE"' "$rs" && grep -q 'exit 2' "$rs"; then
    ok "契约 4：restore 对缺失文件非零退出"
  else
    bad "契约 4：restore 未拒绝缺失文件"
  fi
  # 窄豁免 + 退出码必须被捕获（不是被 || true 吞掉）
  if grep -q 'cannot drop inherited constraint' "$rs"      && grep -q 'RC=\$?' "$rs"      && ! grep -qE 'pg_restore .*\|\| true' "$rs"; then
    ok "契约 5：restore 豁免窄（仅分区表继承约束）+ 退出码被捕获（未吞错）"
  else
    bad "契约 5：restore 缺窄豁免，或 pg_restore 的退出码被吞"
  fi
}

echo "=== 备份/恢复封装契约（E2E-29 M4）==="
check_static "$BK" "$RS"

# ---- 行为负向对照（真跑，不需要数据库）----
echo "--- 行为负向对照 ---"
bash "$RS" >/dev/null 2>&1
rc_missing_arg=$?
if [ "$rc_missing_arg" -ne 0 ]; then
  ok "行为 1：无参数调用 restore → 退出码 $rc_missing_arg（非 0）"
else
  bad "行为 1：无参数调用 restore 竟退出 0"
fi
bash "$RS" "$SCRIPT_DIR/__definitely_not_exist__.dump" >/dev/null 2>&1
rc_no_file=$?
if [ "$rc_no_file" -ne 0 ]; then
  ok "行为 2：不存在的备份文件 → 退出码 $rc_no_file（非 0）"
else
  bad "行为 2：不存在的备份文件竟退出 0"
fi
TMPF="$(mktemp 2>/dev/null || echo "${TEMP:-/tmp}/empty-dump-$$")"
: > "$TMPF"
bash "$RS" "$TMPF" >/dev/null 2>&1
rc_empty=$?
rm -f "$TMPF"
if [ "$rc_empty" -ne 0 ]; then
  ok "行为 3：空备份文件 → 退出码 $rc_empty（非 0）"
else
  bad "行为 3：空备份文件竟退出 0"
fi

# ---- 静态负向对照：把空文件检查删掉，契约 3 必须转红 ----
echo "--- 静态负向对照（副本删掉空文件检查）---"
TMPD="$(mktemp -d 2>/dev/null || echo "${TEMP:-/tmp}/bk-guard-$$")"
mkdir -p "$TMPD"
python - "$BK" "$TMPD/backup_db.sh" <<'PYEOF'
import sys
src, dst = sys.argv[1], sys.argv[2]
t = open(src, encoding="utf-8").read()
t = t.replace('if [ ! -s "$FILE" ]; then', 'if false; then')
t = t.replace('exit 4', 'exit 0')
open(dst, "w", encoding="utf-8").write(t)
PYEOF
cp "$RS" "$TMPD/restore_db.sh"
mutated_ok=0
if grep -q '! -s "\$FILE"' "$TMPD/backup_db.sh" && grep -q 'exit 4' "$TMPD/backup_db.sh"; then
  mutated_ok=1
fi
if [ "$mutated_ok" -eq 0 ]; then
  ok "负向对照：删掉空文件检查后契约 3 判定会转红（守卫有牙齿）"
else
  bad "负向对照失效（副本未被改造）"
fi
rm -rf "$TMPD"

echo
echo "=== Result: $pass passed, $fail failed ==="
[ "$fail" -eq 0 ] || exit 1
exit 0

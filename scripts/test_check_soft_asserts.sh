#!/usr/bin/env bash
# scripts/test_check_soft_asserts.sh —— check_soft_asserts.sh 的负向测试
#
# 为什么需要（R-03 #7 / AP-13）：门禁脚本本身也会写错。
#   只验证"在干净仓库里退出 0"是不够的 —— 一个永远退出 0 的脚本也能通过那种验证。
#   必须证明它在**有缺陷的输入**下会红。
#
# 用例：
#   ① 未登记的 soft-assert（TS）→ 必须 RED
#   ② 未登记的 soft-assert（Go）→ 必须 RED
#   ③ 已登记的 soft-assert（进白名单）→ 必须 GREEN
#   ④ 干净的测试文件（无 soft-assert）→ 必须 GREEN
#   ⑤ 注释里的普通说明（非断言）→ 不得误报
#
# 用法：bash scripts/test_check_soft_asserts.sh
# 退出码：0 = 全部用例通过；1 = 有用例失败

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCANNER="$SCRIPT_DIR/check_soft_asserts.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

pass=0
fail=0

# run_case <名称> <期望退出码> <目标路径> [白名单文件]
run_case() {
  local name="$1" want="$2" target="$3" allow="${4:-}"
  local out rc
  out="$(SOFT_ASSERT_TARGETS="$target" SOFT_ASSERT_ALLOWLIST_OVERRIDE="$allow" bash "$SCANNER" 2>&1)"
  rc=$?
  if [ "$rc" = "$want" ]; then
    echo "  [PASS] $name（exit=$rc，期望 $want）"
    pass=$((pass + 1))
  else
    echo "  [FAIL] $name：exit=$rc，期望 $want"
    echo "$out" | sed 's/^/         /' | head -8
    fail=$((fail + 1))
  fi
}

echo "=== check_soft_asserts.sh 负向/正向测试 ==="

# ① 未登记 TS soft-assert
cat > "$TMP/dirty.spec.ts" <<'EOF'
import { test, expect } from '@playwright/test'
test('x', async () => {
  // expect(critical).toEqual([])
})
EOF
run_case "① 未登记 soft-assert（TS）应 RED" 1 "$TMP/dirty.spec.ts"

# ② 未登记 Go soft-assert
cat > "$TMP/dirty_test.go" <<'EOF'
package foo

import "testing"

func TestX(t *testing.T) {
	// require.NoError(t, err)
}
EOF
run_case "② 未登记 soft-assert（Go）应 RED" 1 "$TMP/dirty_test.go"

# ③ 干净的测试文件
cat > "$TMP/clean_test.go" <<'EOF'
package foo

import "testing"

func TestX(t *testing.T) {
	if 1 != 1 {
		t.Fatal("unreachable")
	}
}
EOF
run_case "③ 干净测试文件应 GREEN" 0 "$TMP/clean_test.go"

# ④ 注释里的普通说明不得误报
cat > "$TMP/comment_only.spec.ts" <<'EOF'
import { test, expect } from '@playwright/test'
test('x', async () => {
  // 这里曾经有一个 expect(...) 断言，已移到别的用例
  // 记录 violations
})
EOF
run_case "④ 普通注释不得误报" 0 "$TMP/comment_only.spec.ts"

# ⑤ 白名单覆盖（只对 check_soft_asserts 的白名单文件生效的路径做等价验证）
#    说明：白名单以"路径:行号"为键，故这里用仓库内**已登记**的那条做正向验证。
if [ -f "$SCRIPT_DIR/soft_assert_allowlist.txt" ]; then
  known_key="$(grep -vE '^[[:space:]]*(#|$)' "$SCRIPT_DIR/soft_assert_allowlist.txt" | awk '{print $1}' | head -1)"
  known_file="${known_key%:*}"
  if [ -n "$known_file" ] && [ -f "$SCRIPT_DIR/../$known_file" ]; then
    run_case "⑤ 仓库内已登记项应 GREEN（$known_key）" 0 "$SCRIPT_DIR/../$known_file"
  fi
fi

echo ""
echo "==========="
echo "PASS=$pass FAIL=$fail"
echo "==========="
[ "$fail" -gt 0 ] && exit 1
exit 0

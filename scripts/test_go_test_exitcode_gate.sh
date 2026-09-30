#!/usr/bin/env bash
# test_go_test_exitcode_gate.sh —— 防「用失真的命令判定 go test 结果」
#
# 背景（2026-09-30，E2E-23 收口轮亲历）：
#   我在 5 个 Go 服务上跑完测试后，用
#       r=$(go test ./... -count=1 2>&1 | grep -cE "^FAIL"); echo "失败数=$r"
#   得到 **全部为 0**，据此写下"5 个服务全绿"，并在此基础上把阶段判成 `done`。
#   **这是假的。** 实际有 9 个测试失败。
#
#   真因：**本机 Git Bash 的 grep 是 ugrep 7.8.4**，
#   `grep -cE "^FAIL"` 对该输入返回 **0**，而 `grep -c "FAIL"` 返回 8。
#   即**带 `^` 锚点的匹配在这台机器上失效**。
#   同一个 `grep -c` 家族还有第二个已知失效形态（见 memory `pipefail-grep-q-sigpipe-gate-trap`：
#   "命中即退出 → 左侧 SIGPIPE → pipefail 返回非零 → 判假"）。
#
#   两个后果都严重：
#   ① 我据此**误判阶段可以 done**；
#   ② 真正抓住问题的是 **CI**（它跑 `go test -race -count=1 ./...`，用退出码判定），
#      而不是我的本地校验。**门禁在，本地校验反而是假绿的那一环。**
#
# 本守卫的作用不是"测试服务"（那由 go-test workflow 做），
# 而是**锁死"判定 go test 结果必须用退出码"这条规则**，并把上述失效形态写成断言。
#
# 用法：bash scripts/test_go_test_exitcode_gate.sh
# 退出码：0 = 通过；1 = 有 FAIL

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

PASS=0; FAIL=0
ok()  { PASS=$((PASS+1)); echo "  PASS  $1"; }
bad() { FAIL=$((FAIL+1)); echo "  FAIL  $1"; }

echo "== go test 结果判定方式的回归钉 =="

# ---------------------------------------------------------------- 1. 锚点 grep 在本机是否失效
echo
echo "-- 1. 记录本机 grep 对 '^' 锚点的实际行为（这是本轮踩坑的直接原因） --"
tmp="$(mktemp)"
printf 'ok  \tpkg\t1.0s\n--- FAIL: TestX (0.00s)\nFAIL\nFAIL\tpkg\t0.5s\n' > "$tmp"
anchored="$(grep -cE '^FAIL' "$tmp" 2>/dev/null || echo 0)"
plain="$(grep -c 'FAIL' "$tmp" 2>/dev/null || echo 0)"
rm -f "$tmp"

if [ "$plain" -ge 2 ]; then
  ok "grep 本身可用（无锚点匹配到 $plain 处 FAIL）"
else
  bad "grep 连无锚点都匹配不到（$plain）—— 本机 grep 环境异常，判定结果不可信"
fi
if [ "$anchored" -lt "$plain" ]; then
  echo "  NOTE  ⚠️ 本机 \`grep -cE '^FAIL'\` 返回 $anchored，而 \`grep -c 'FAIL'\` 返回 $plain"
  echo "  NOTE     ⇒ **带 ^ 锚点的 grep 匹配在本机失真**（实测：Git Bash 的 grep 是 ugrep）。"
  echo "  NOTE     这正是本轮误报'5 个服务全绿'的直接原因；本守卫的存在就是为了"
  echo "  NOTE     记住：**不要再用 grep 计数来判定 go test 的成败**。"
else
  ok "带 ^ 锚点的 grep 行为正常（$anchored）"
fi

# ---------------------------------------------------------------- 2. 正确的判定方式可用
echo
echo "-- 2. 正确判定方式：直接用 go test 的**退出码** --"
if ! command -v go >/dev/null 2>&1; then
  bad "找不到 go，本条无法验证"
else
  (cd emotion-echo-shared && go test ./pkg/healthcheck/ -count=1 >/dev/null 2>&1)
  rc_ok=$?
  if [ $rc_ok -eq 0 ]; then
    ok "通过用例：go test 退出码 0（healthcheck 包全绿）"
  else
    bad "通过用例竟然非零（rc=$rc_ok）"
  fi

  # 反例：必然失败的用例必须非零
  tmpd="$(mktemp -d)"
  cat > "$tmpd/x_test.go" <<'GOEOF'
package tmpguard

import "testing"

func TestMustFail(t *testing.T) { t.Fatal("deliberate failure") }
GOEOF
  cat > "$tmpd/go.mod" <<'GOEOF'
module tmpguard

go 1.26
GOEOF
  (cd "$tmpd" && go test ./... >/dev/null 2>&1)
  rc_bad=$?
  rm -rf "$tmpd"
  if [ $rc_bad -ne 0 ]; then
    ok "反例：故意失败的用例退出码非零（rc=$rc_bad）⇒ 退出码判定可靠"
  else
    bad "故意失败的用例退出码竟为 0 ⇒ 退出码判定在此环境失效"
  fi
fi

# ---------------------------------------------------------------- 3. 文档级断言
echo
echo "-- 3. 仓库内不得再用 grep 计数判定 go test 成败 --"
hits="$(grep -rn "go test.*| *grep -c" --include=*.sh --include=*.md --include=*.py . 2>/dev/null \
        | grep -v 'test_go_test_exitcode_gate.sh' || true)"
if [ -z "$hits" ]; then
  ok "scripts/ 与文档中无 'go test | grep -c' 形态的判定"
else
  bad "仍存在该形态：$(printf '%s' "$hits" | tr '\n' ' ' | cut -c1-160)"
fi

echo
echo "PASS: $PASS  FAIL: $FAIL"
if [ "$FAIL" -gt 0 ]; then
  echo "RED：判定 go test 结果的方式不可靠 —— 会把红读成绿"
  exit 1
fi
echo "GREEN：go test 结果一律用退出码判定；已知 ugrep 锚点失效形态已记录在案"
exit 0

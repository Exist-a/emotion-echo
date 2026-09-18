#!/usr/bin/env bash
# scripts/check_soft_asserts.sh —— soft-assert（被注释掉的断言）扫描
#
# 为什么需要（RUNBOOK §13.3 断言 #8 / 反例 AP-01）：
#   把断言注释掉，测试就**永远不会失败**，于是"测试通过"变成假绿。
#   E2E-04 的 a11y 基线就踩过：`expect(critical).toEqual([])` 被注释，
#   report 却记"a11y 基线跑出结果"（账本 E2E-F-51）。
#   这类"永远不会红的测试"比没有测试更危险——它提供虚假的安全感。
#
# 判定：扫描测试文件里以注释形式出现的断言调用。
#   - 命中且**在** 白名单 → WARN（已知缺口，必须写明理由与账本编号）
#   - 命中且**不在**白名单 → FAIL
# 白名单文件：scripts/soft_assert_allowlist.txt
#   格式：<相对路径>:<行号>  # <理由 / 账本编号>
#   用"路径:行号"而非仅路径，避免同一文件新增的 soft-assert 被旧条目顺带放过。
#
# 用法：bash scripts/check_soft_asserts.sh
# 退出码：0 = 无未登记 soft-assert；1 = 有未登记项

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ALLOWLIST="$SCRIPT_DIR/soft_assert_allowlist.txt"

cd "$REPO_ROOT"

# 扫描范围：默认扫前端 e2e/vitest 与各 Go 模块测试。
# SOFT_ASSERT_TARGETS 可覆盖（空格分隔的目录或文件）——供 scripts/test_check_soft_asserts.sh
# 用临时 fixture 做负向测试，无需改动仓库内容。
DEFAULT_TARGETS=(
  emotion-echo-web/e2e emotion-echo-web/app emotion-echo-web/tests
  emotion-echo-web-bff emotion-echo-user-svc emotion-echo-chat-svc
  emotion-echo-analytics-svc emotion-echo-assessment-svc emotion-echo-ai-svc
  emotion-echo-shared
)
# shellcheck disable=SC2206
if [ -n "${SOFT_ASSERT_TARGETS:-}" ]; then
  TARGETS=(${SOFT_ASSERT_TARGETS})
else
  TARGETS=("${DEFAULT_TARGETS[@]}")
fi

# 注释掉的断言形态：行首（可含缩进）为 // 或 /* 或 *，紧跟断言调用
TS_RE='^[[:space:]]*(//|/\*|\*)[[:space:]]*(expect[[:space:]]*\(|assert\.[a-zA-Z]|assertThat)'
GO_RE='^[[:space:]]*//[[:space:]]*(assert\.|require\.|t\.Error|t\.Fatal)'
ANY_RE="($TS_RE)|($GO_RE)"

hits_file="$(mktemp)"
: > "$hits_file"

for t in "${TARGETS[@]}"; do
  [ -e "$t" ] || continue
  if [ -f "$t" ]; then
    files=("$t")
  else
    files=()
    while IFS= read -r f; do [ -n "$f" ] && files+=("$f"); done < <(
      find "$t" -type f \( -name '*.spec.ts' -o -name '*.test.ts' -o -name '*_test.go' \) 2>/dev/null || true
    )
  fi
  for f in "${files[@]:-}"; do
    [ -n "$f" ] || continue
    grep -nE "$ANY_RE" "$f" 2>/dev/null | while IFS=: read -r ln rest; do
      printf '%s:%s\n' "$f" "$ln" >> "$hits_file"
    done
  done
done

sort -u "$hits_file" -o "$hits_file"
total=$(wc -l < "$hits_file" | tr -d ' ')

echo "=== soft-assert 扫描 ==="
echo "命中（注释形式的断言）: $total"

allow_file="/dev/null"
[ -f "$ALLOWLIST" ] && allow_file="$ALLOWLIST"
allow_keys="$(grep -vE '^[[:space:]]*(#|$)' "$allow_file" 2>/dev/null | awk '{print $1}' | sort -u || true)"
allow_count=$(printf '%s\n' "$allow_keys" | grep -c . || true)
echo "白名单条目: $allow_count（$ALLOWLIST）"
echo ""

unregistered=0
while IFS= read -r hit; do
  [ -n "$hit" ] || continue
  # 归一化：把"文件:行号"里的文件解析成**仓库相对路径** ——
  # 白名单键一律用仓库相对路径。若只做字符串替换，`scripts/../x.spec.ts:51`
  # 这类带中间 .. 的形式会匹配不上，出现"明明登记了还报红"的假失败。
  file_part="${hit%:*}"
  line_part="${hit##*:}"
  rel="$(realpath --relative-to="$REPO_ROOT" "$file_part" 2>/dev/null || printf '%s' "$file_part")"
  norm="$(printf '%s:%s' "$rel" "$line_part" | tr '\\' '/')"
  if printf '%s\n' "$allow_keys" | grep -qxF "$norm"; then
    echo "  [WARN] $norm — 已登记已知缺口（见白名单理由）"
  else
    echo "  [FAIL] $norm — 未登记的 soft-assert（断言被注释 ⇒ 该测试永不失败）"
    unregistered=$((unregistered + 1))
  fi
done < "$hits_file"

echo ""
if [ "$unregistered" -gt 0 ]; then
  echo "RED: $unregistered 处未登记的 soft-assert。"
  echo "     修法二选一：① 恢复断言；② 若确认暂不强制，登记进 $ALLOWLIST 并写明理由 + 账本编号。"
  rm -f "$hits_file"
  exit 1
fi

echo "GREEN: 无未登记的 soft-assert（已登记的已知缺口见上方 WARN）"
rm -f "$hits_file"
exit 0
